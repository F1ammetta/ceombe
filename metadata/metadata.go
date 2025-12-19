package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	// "sort"
	"strconv"
	"strings"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"

	"github.com/bogem/id3v2/v2"
)

// CoverArtInfo is the unmarshaled representation of a JSON file in the Cover Art Archive.
// See https://musicbrainz.org/doc/Cover_Art_Archive/API#Cover_Art_Archive_Metadata for an example.
type CoverArtInfo struct {
	Images  []CoverArtImageInfo
	Release string
}

// CoverArtImageInfo is the unmarshaled representation of a single images metadata in a CAA JSON file.
// See https://musicbrainz.org/doc/Cover_Art_Archive/API#Cover_Art_Archive_Metadata for an example.
type CoverArtImageInfo struct {
	Approved   bool
	Back       bool
	Comment    string
	Edit       int
	Front      bool
	ID         int
	Image      string
	Thumbnails ThumbnailMap
	Types      []string
}

// CoverArtImage is a wrapper around an image from the CAA, containing its binary data and mimetype information.
type CoverArtImage struct {
	Data     []byte
	Mimetype string
}

// ThumbnailMap maps thumbnail names to their URLs. The only valid keys are
// "large" and "small", "250", "500" and "1200".
type ThumbnailMap map[string]string

type AcoustIDRequest struct {
	Fingerprint string `json:"fingerprint"`
	Duration    int    `json:"duration"`
	ApiKey      string `json:"client"`
	Metadata    string `json:"meta"`
}

// Define a struct to hold our best match for clarity
type BestMatch struct {
	Recording    ResultRecording
	Artist       ResultArtist
	ReleaseGroup ResultReleaseGroup
	Score        float64
}

// To use the anonymous structs from the response, we'll give them names.
// This makes the code much cleaner.
type ResultArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ResultReleaseGroup struct {
	Type           string   `json:"type"`
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	SecondaryTypes []string `json:"secondarytypes"`
}

type ResultRecording struct {
	Artists       []ResultArtist       `json:"artists"`
	ReleaseGroups []ResultReleaseGroup `json:"releasegroups"`
	Duration      float64              `json:"duration"`
	ID            string               `json:"id"`
	Title         string               `json:"title"`
}

type Result struct {
	ID         string            `json:"id"`
	Recordings []ResultRecording `json:"recordings"`
	Score      float64           `json:"score"`
}

// Let's update the AcoustIDResponse to use these named structs
type AcoustIDResponse struct {
	Results []Result `json:"results"`
	Status  string   `json:"status"`
}

// selectBestReleaseGroup chooses the best album from a list, prioritizing by type
// and penalizing releases that appear to be compilations based on their title.
func selectBestReleaseGroup(releaseGroups []ResultReleaseGroup, title string, artist string) *ResultReleaseGroup {
	if len(releaseGroups) == 0 {
		return nil
	}

	var bestReleaseGroup *ResultReleaseGroup
	// Use a very low starting score to correctly handle negative scores from penalties.
	maxScore := -1000

	// Define the preference for release types. Higher score is better.
	typeScores := map[string]int{
		"Album":       4,
		"EP":          3,
		"Single":      2,
		"Compilation": 1, // Already has a low score, will be penalized further if title matches.
	}

	// Expanded keywords that indicate a compilation or party album.
	compilationKeywords := []string{
		"hits", "greatest", "best of", "the best", "anthology", "hit", "latin", "directo", "vivo", "40", "fm", "exitos", "éxitos", "principales", "grandes", "audifonos", "audífonos",
		"essentials", "collection", "very best", "ultimate", "party", // "party" added.
	}

	// NEW: Regex to find 4-digit numbers which might be years.
	yearRegex := regexp.MustCompile(`\b\d{4}\b`)
	currentYear := time.Now().Year()

	for i, rg := range releaseGroups {
		score, ok := typeScores[rg.Type]
		if !ok {
			score = 0 // Give a low score to other types like "Broadcast", etc.
		}

		penaltyApplied := false
		lowerCaseTitle := strings.ToLower(rg.Title)

		// Check #1: Scan for compilation keywords.
		for _, keyword := range compilationKeywords {
			if strings.Contains(lowerCaseTitle, keyword) {
				score -= 10 // Apply penalty.
				penaltyApplied = true
				break
			}
		}

		// NEW: Check #2: If no keyword was found, scan for a year number.
		if !penaltyApplied {
			potentialYears := yearRegex.FindAllString(rg.Title, -1)
			for _, yearStr := range potentialYears {
				year, err := strconv.Atoi(yearStr)
				if err == nil {
					// Check if the number is a plausible year for a music compilation.
					if year >= 1950 && year <= currentYear+1 {
						score -= 10 // Apply same penalty.
						break
					}
				}
			}
		}

		if strings.Contains(lowerCaseTitle, strings.ToLower(title)) {
			score += 15
		}

		if strings.Contains(lowerCaseTitle, strings.ToLower(artist)) {
			score += 15
		}

		if score > maxScore {
			maxScore = score
			// We have to point to the address of the item in the slice.
			bestReleaseGroup = &releaseGroups[i]
		}
	}

	if bestReleaseGroup == nil && len(releaseGroups) > 0 {
		return &releaseGroups[0]
	}

	return bestReleaseGroup
}

func checkCoverArtExists(albumId string) bool {
	res, err := http.Get("https://coverartarchive.org/release-group/" + albumId)
	if err != nil {
		// Any network error (DNS, timeout, etc.) is treated as "no cover art found".
		return false
	}
	defer res.Body.Close()

	// A 200 OK status code means a metadata document was found, which implies images are available.
	// Any other status (404, 500, etc.) is treated as no cover art.
	return res.StatusCode == http.StatusOK
}

// findBestRecording iterates through all recordings to find the best match.
func findBestRecording(results []Result, titleFromFile string, artistFromFile string) *BestMatch {
	var bestMatch *BestMatch
	maxScore := -1.0

	jw := metrics.NewJaroWinkler()
	jw.CaseSensitive = false

	// Define a bonus to be added to the similarity score if cover art is available.
	// This should be high enough to outweigh small differences in text similarity.
	const coverArtBonus = 0.15

	for _, result := range results {
		for _, recording := range result.Recordings {
			// A recording must have an artist and an album to be considered.
			if len(recording.Artists) == 0 || len(recording.ReleaseGroups) == 0 {
				continue
			}

			// First, determine the best album for this recording based on type (Album > EP > etc.)
			// This is a quick, I/O-free operation.
			bestAlbumForThisRecording := selectBestReleaseGroup(recording.ReleaseGroups, titleFromFile, artistFromFile)
			if bestAlbumForThisRecording == nil {
				continue // Should not happen given the check above, but for safety.
			}

			// Now, perform a single network check to see if this album has cover art.
			// This is the key change to prioritize albums with art.
			hasCoverArt := checkCoverArtExists(bestAlbumForThisRecording.ID)

			for _, artist := range recording.Artists {
				titleScore := strutil.Similarity(titleFromFile, recording.Title, jw)
				artistScore := strutil.Similarity(artistFromFile, artist.Name, jw)

				// Weighted score: 60% for artist match, 40% for title match.
				currentScore := (artistScore * 0.6) + (titleScore * 0.4)

				// Add the bonus if cover art was found.
				if hasCoverArt {
					currentScore += coverArtBonus
				}

				if currentScore > maxScore {
					maxScore = currentScore
					bestMatch = &BestMatch{
						Recording:    recording,
						Artist:       artist,
						ReleaseGroup: *bestAlbumForThisRecording,
						Score:        currentScore,
					}
				}
			}
		}
	}

	return bestMatch
}

func (a *AcoustIDRequest) Do() (*AcoustIDResponse, error) {
	client := http.Client{}
	response, err := client.PostForm("https://api.acoustid.org/v2/lookup", a.PostValues())
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var aidResp AcoustIDResponse
	if err := json.Unmarshal(body, &aidResp); err != nil {
		return nil, err
	}

	return &aidResp, nil
}

func (a *AcoustIDRequest) PostValues() url.Values {
	values, _ := url.ParseQuery(fmt.Sprintf(
		"client=%s&duration=%d&meta=%s&fingerprint=%s",
		a.ApiKey,
		a.Duration,
		a.Metadata,
		a.Fingerprint))
	return values
}

func GetFingerprint(filePath string) (string, int, error) {
	execPath, err := exec.LookPath("fpcalc")
	if err != nil {
		return "", -1, err
	}

	cmd := exec.Command(execPath, filePath)
	output, err := cmd.Output()
	if err != nil {
		return "", -1, err
	}

	str := strings.Split(string(output), "=")
	if len(str) < 2 {
		return "", -1, fmt.Errorf("Invalid output")
	}
	fingerprint := str[len(str)-1]
	fingerprint = strings.TrimSpace(fingerprint)

	durations := strings.Split(str[1], "\n")[0]
	durations = strings.TrimSpace(durations)

	duration, err := strconv.Atoi(durations)

	if err != nil {
		return "", -1, err
	}

	return fingerprint, duration, nil
}

func GetCoverImage(albumId string) ([]byte, error) {
	res, err := http.Get("https://coverartarchive.org/release-group/" + albumId)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover art not found: status code %d", res.StatusCode)
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var data CoverArtInfo
	if err := json.Unmarshal(resBody, &data); err != nil {
		return nil, err
	}

	if len(data.Images) == 0 {
		return nil, fmt.Errorf("no cover image found in response")
	}

	imageURL := ""
	if val, ok := data.Images[0].Thumbnails["1200"]; ok {
		imageURL = val
	} else if val, ok := data.Images[0].Thumbnails["large"]; ok {
		imageURL = val
	} else if val, ok := data.Images[0].Thumbnails["500"]; ok {
		imageURL = val
	} else {
		return nil, fmt.Errorf("no suitable thumbnail found")
	}

	res, err = http.Get(imageURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	return io.ReadAll(res.Body)
}

type Metadata struct {
	Title  string
	Artist string
	Album  string
}

func parseFilename(filePath string) (title string, artist string) {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	parts := strings.Split(name, " - ")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

func GetMetadata(filePath string) (*Metadata, error) {
	fingerprint, duration, err := GetFingerprint(filePath)
	if err != nil {
		return nil, fmt.Errorf("error getting fingerprint: %w", err)
	}

	request := AcoustIDRequest{
		Fingerprint: fingerprint,
		Duration:    duration,
		ApiKey:      "djeyw3pqpz", // TODO: Move to config
		Metadata:    "recordings+releasegroups+compress",
	}

	response, err := request.Do()
	if err != nil {
		return nil, fmt.Errorf("error from AcoustID: %w", err)
	}

	if len(response.Results) == 0 {
		return nil, fmt.Errorf("no results found from AcoustID")
	}

	// 1. Parse the title and artist from the local filename
	titleFromFile, artistFromFile := parseFilename(filePath)
	if artistFromFile == "" {
		return nil, fmt.Errorf("could not parse artist from filename: %s", filePath)
	}

	// 2. Find the single best recording across all results
	bestMatch := findBestRecording(response.Results, titleFromFile, artistFromFile)
	if bestMatch == nil {
		return nil, fmt.Errorf("no suitable recording match found in AcoustID results")
	}

	// 3. Use the metadata from the best match
	Title := bestMatch.Recording.Title
	Artist := bestMatch.Artist.Name
	Album := bestMatch.ReleaseGroup.Title
	AlbumId := bestMatch.ReleaseGroup.ID

	image, _ := GetCoverImage(AlbumId) // Error is ignored, as cover art is optional

	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return nil, fmt.Errorf("failed to open file for tagging: %w", err)
	}
	defer tag.Close()

	tag.SetTitle(Title)
	tag.SetArtist(Artist)
	tag.SetAlbum(Album)

	if image != nil {
		pic := id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    "image/jpeg",
			PictureType: id3v2.PTFrontCover,
			Description: "Front cover",
			Picture:     image,
		}
		tag.AddAttachedPicture(pic)
	}

	if err = tag.Save(); err != nil {
		return nil, fmt.Errorf("failed to save tags: %w", err)
	}

	return &Metadata{
		Title:  Title,
		Artist: Artist,
		Album:  Album,
	}, nil
}

func ReadTagsFromFile(filePath string) (*Metadata, error) {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return nil, fmt.Errorf("failed to open file to read tags: %w", err)
	}
	defer tag.Close()

	return &Metadata{
		Title:  tag.Title(),
		Artist: tag.Artist(),
		Album:  tag.Album(),
	}, nil
}

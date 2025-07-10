package metadata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

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

func check(err error) {
	if err != nil {
		panic(err)
	}
}

type AcoustIDRequest struct {
	Fingerprint string `json:"fingerprint"`
	Duration    int    `json:"duration"`
	ApiKey      string `json:"client"`
	Metadata    string `json:"meta"`
}

type Result struct {
	ID string `json:"id"`

	Recordings []struct {
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`

		ReleaseGroups []struct {
			Type           string   `json:"type"`
			ID             string   `json:"id"`
			Title          string   `json:"title"`
			SecondaryTypes []string `json:"secondarytypes"`
		} `json:"releasegroups"`

		Duration float64 `json:"duration"`
		ID       string  `json:"id"`
		Title    string  `json:"title"`
	} `json:"recordings"`

	Score float64 `json:"score"`
}

type AcoustIDResponse struct {
	Results []Result `json:"results"`
	Status  string   `json:"status"`
}

func (a *AcoustIDRequest) Do() AcoustIDResponse {
	client := http.Client{}
	response, err := client.PostForm("https://api.acoustid.org/v2/lookup", a.PostValues())
	check(err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)

	aidResp := AcoustIDResponse{}
	err = json.Unmarshal(body, &aidResp)
	check(err)

	return aidResp
}

func (a *AcoustIDRequest) PostValues() url.Values {
	query := fmt.Sprintf(
		"client=%s&duration=%d&meta=%s&fingerprint=%s",
		a.ApiKey,
		a.Duration,
		a.Metadata,
		a.Fingerprint)

	values, err := url.ParseQuery(query)
	check(err)
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

	durations := strings.Split(str[1], "
")[0]
	durations = strings.TrimSpace(durations)

	duration, err := strconv.Atoi(durations)

	if err != nil {
		return "", -1, err
	}

	return fingerprint, duration, nil
}

func GetCoverImage(albumId string) []byte {
	// fetch cover image
	res, err := http.Get("https://coverartarchive.org/release-group/" + albumId)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer res.Body.Close()

	// print body
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	var data CoverArtInfo

	err = json.Unmarshal(resBody, &data)

	if err != nil {
		fmt.Println(err)
		return nil
	}

	if len(data.Images) == 0 {
		fmt.Println("No cover image found")
		return nil
	}

	image_url := data.Images[0].Thumbnails["1200"]

	// get image data and return
	res, err = http.Get(image_url)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	defer res.Body.Close()

	imageData, err := io.ReadAll(res.Body)

	if err != nil {
		fmt.Println(err)
		return nil
	}

	return imageData
}

type Metadata struct {
	Title  string
	Artist string
	Album  string
}

func GetMetadata(filePath string) (*Metadata, error) {
	fingerprint, duration, err := GetFingerprint(filePath)
	if err != nil {
		return nil, err
	}

	request := AcoustIDRequest{
		Fingerprint: fingerprint,
		Duration:    int(duration),
		ApiKey:      "djeyw3pqpz",
		Metadata:    "recordings+releasegroups+compress",
	}

	response := request.Do()

	if len(response.Results) == 0 {
		return nil, fmt.Errorf("No results found")
	}

	rec := response.Results[0].Recordings[0]
	Title := rec.Title
	Artist := rec.Artists[0].Name
	Album := rec.ReleaseGroups[0].Title
	AlbumId := rec.ReleaseGroups[0].ID

	image := GetCoverImage(AlbumId)

	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return nil, err
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
		return nil, err
	}

	return &Metadata{
		Title:  Title,
		Artist: Artist,
		Album:  Album,
	}, nil
}
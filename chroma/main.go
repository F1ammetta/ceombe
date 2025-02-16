package main

import (
	"encoding/json"
	"fmt"
	"github.com/bogem/id3v2/v2"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

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

func main() {
	args := os.Args

	if len(args) < 2 {
		fmt.Println("Please provide a file path")
		return
	}

	filePath := args[1]

	// call fpcalc to get fingerprint
	fingerprint, duration, err := GetFingerprint(filePath)

	if err != nil {
		fmt.Println(err)
		return
	}

	request := AcoustIDRequest{
		Fingerprint: fingerprint,
		Duration:    int(duration),
		ApiKey:      "djeyw3pqpz",
		Metadata:    "recordings+releasegroups+compress",
	}

	response := request.Do()

	if len(response.Results) == 0 {
		fmt.Println("No results found")
		return
	}

	Title := response.Results[0].Recordings[0].Title
	Artist := response.Results[0].Recordings[0].Artists[0].Name
	Album := response.Results[0].Recordings[0].ReleaseGroups[0].Title
	AlbumId := response.Results[0].Recordings[0].ReleaseGroups[0].ID

	print("Title: " + Title + "\n")
	print("Artist: " + Artist + "\n")
	print("Album: " + Album + "\n")

	image := GetCoverImage(AlbumId)

	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})

	if err != nil {
		fmt.Println(err)
		return
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
		fmt.Println(err)
		return
	}
}

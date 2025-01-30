package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bogem/id3v2/v2"
)

func GetFingerprint(filePath string) (string, int, error) {
	execPath, err := exec.LookPath("/usr/bin/fpcalc")
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
		fmt.Println("error making request")
		fmt.Println(err)
		return nil
	}
	defer res.Body.Close()

	// print body
	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("unable to read response data")
		fmt.Println(err)
		return nil
	}

	var data CoverArtInfo

	err = json.Unmarshal(resBody, &data)

	if err != nil {
		fmt.Println("unable to unmarshal response")
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
		fmt.Println("unable to retreive image")
		fmt.Println(err)
		return nil
	}

	defer res.Body.Close()

	imageData, err := io.ReadAll(res.Body)

	if err != nil {
		fmt.Println("unable to read image data")
		fmt.Println(err)
		return nil
	}

	return imageData
}

func main() {
	args := os.Args

	// fmt.Printf("%v\n", args)

	fmt.Println(len(args))

	// if len(args) < 2 {
	// 	fmt.Println("Please provide a file path")
	// 	return
	// }

	filePath := args[1]

	fmt.Println("getting fringerprint")
	// call fpcalc to get fingerprint
	fingerprint, duration, err := GetFingerprint(filePath)
	fmt.Println("fringerprint success")

	if err != nil {
		fmt.Println("getting fingered")
		fmt.Println(err)
		return
	}

	request := AcoustIDRequest{
		Fingerprint: fingerprint,
		Duration:    int(duration),
		ApiKey:      "djeyw3pqpz",
		Metadata:    "recordings+releasegroups+compress",
	}

	fmt.Println("making request to acoustid")
	response := request.Do()
	fmt.Println("request success")

	if len(response.Results) == 0 {
		fmt.Println("No results found")
		return
	}

	var index int
	maxScore := 0.0

	for i, result := range response.Results {
		if result.Score > maxScore {
			maxScore = result.Score
			index = i
		}
	}

	Title := response.Results[index].Recordings[0].Title
	Artist := response.Results[index].Recordings[0].Artists[0].Name
	Album := response.Results[index].Recordings[0].ReleaseGroups[0].Title
	AlbumId := response.Results[index].Recordings[0].ReleaseGroups[0].ID

	print("Title: " + Title + "\n")
	print("Artist: " + Artist + "\n")
	print("Album: " + Album + "\n")

	fmt.Println("getting image")
	image := GetCoverImage(AlbumId)
	fmt.Println("got image")

	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})

	if err != nil {
		fmt.Println("unable to open file")
		fmt.Println(err)
		return
	}

	defer tag.Close()

	fmt.Println("setting tags")

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

		fmt.Println("adding pic")

		tag.AddAttachedPicture(pic)
	}

	fmt.Println("saving")
	if err = tag.Save(); err != nil {
		fmt.Println("unable to write to file")
		fmt.Println(err)
		return
	}
}

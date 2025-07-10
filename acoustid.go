package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
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

	body, err := ioutil.ReadAll(response.Body)

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

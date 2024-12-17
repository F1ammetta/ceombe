package subsonic

import (
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// GetStreamUrl returns the Stream Url contents of a song, optionally transcoded, from the server.
//
// Optional Parameters:
//
//	maxBitRate:             (Since 1.2.0) If specified, the server will attempt to limit the bitrate to this value, in kilobits per second. If set to zero, no limit is imposed.
//	format:                 (Since 1.6.0) Specifies the preferred target format (e.g., "mp3" or "flv") in case there are multiple applicable transcodings.  Starting with 1.9.0 you can use the special value "raw" to disable transcoding.
//	timeOffset:             Only applicable to video streaming. If specified, start streaming at the given offset (in seconds) into the video. Typically used to implement video skipping.
//	size:                   (Since 1.6.0) Only applicable to video streaming. Requested video size specified as WxH, for instance "640x480".
//	estimateContentLength:  (Since 1.8.0). If set to "true", the Content-Length HTTP header will be set to an estimated value for transcoded or downsampled media.
//	converted:              (Since 1.14.0) Only applicable to video streaming. Subsonic can optimize videos for streaming by converting them to MP4. If a conversion exists for the video in question, then setting this parameter to "true" will cause the converted video to be returned instead of the original.
func (s *Client) GetStreamUrl(id string, parameters map[string]string) (string, error) {
	params := url.Values{}
	params.Add("id", id)
	for k, v := range parameters {
		params.Add(k, v)
	}
	endpoint := "stream"

	baseUrl, err := url.Parse(s.BaseUrl)
	if err != nil {
		return "", err
	}
	baseUrl.Path = path.Join(baseUrl.Path, "/rest/", endpoint)
	req, err := http.NewRequest("get", baseUrl.String(), nil)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	q.Add("f", "xml")
	q.Add("v", supportedApiVersion)
	q.Add("c", s.ClientName)
	q.Add("u", s.User)
	if s.PasswordAuth {
		q.Add("p", s.password)
	} else {
		q.Add("t", s.token)
		q.Add("s", s.salt)
	}

	for key, values := range params {
		for _, val := range values {
			q.Add(key, val)
		}
	}

	req.URL.RawQuery = q.Encode()

	return req.URL.String(), nil
}

// Stream returns the contents of a song, optionally transcoded, from the server.
//
// Optional Parameters:
//
//	maxBitRate:             (Since 1.2.0) If specified, the server will attempt to limit the bitrate to this value, in kilobits per second. If set to zero, no limit is imposed.
//	format:                 (Since 1.6.0) Specifies the preferred target format (e.g., "mp3" or "flv") in case there are multiple applicable transcodings.  Starting with 1.9.0 you can use the special value "raw" to disable transcoding.
//	timeOffset:             Only applicable to video streaming. If specified, start streaming at the given offset (in seconds) into the video. Typically used to implement video skipping.
//	size:                   (Since 1.6.0) Only applicable to video streaming. Requested video size specified as WxH, for instance "640x480".
//	estimateContentLength:  (Since 1.8.0). If set to "true", the Content-Length HTTP header will be set to an estimated value for transcoded or downsampled media.
//	converted:              (Since 1.14.0) Only applicable to video streaming. Subsonic can optimize videos for streaming by converting them to MP4. If a conversion exists for the video in question, then setting this parameter to "true" will cause the converted video to be returned instead of the original.
func (s *Client) Stream(id string, parameters map[string]string) (io.ReadCloser, error) {
	params := url.Values{}
	params.Add("id", id)
	for k, v := range parameters {
		params.Add(k, v)
	}
	response, err := s.Request("GET", "stream", params)
	if err != nil {
		return nil, err
	}
	contentType := response.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/xml") || strings.HasPrefix(contentType, "application/xml") {
		// An error was returned
		responseBody, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		resp := Response{}
		err = xml.Unmarshal(responseBody, &resp)
		if err != nil {
			return nil, err
		}
		if resp.Error != nil {
			err = fmt.Errorf("Error #%d: %s\n", resp.Error.Code, resp.Error.Message)
		} else {
			err = fmt.Errorf("An error occurred: %#v\n", resp)
		}
		return nil, err
	}
	return response.Body, nil
}

// Download returns a given media file. Similar to stream, but this method returns the original media data without transcoding or downsampling.
func (s *Client) Download(id string) (io.ReadCloser, error) {
	params := url.Values{}
	params.Add("id", id)
	response, err := s.Request("GET", "download", params)
	if err != nil {
		return nil, err
	}
	contentType := response.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/xml") || strings.HasPrefix(contentType, "application/xml") {
		// An error was returned
		responseBody, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		resp := Response{}
		err = xml.Unmarshal(responseBody, &resp)
		if err != nil {
			return nil, err
		}
		if resp.Error != nil {
			err = fmt.Errorf("Error #%d: %s\n", resp.Error.Code, resp.Error.Message)
		} else {
			err = fmt.Errorf("An error occurred: %#v\n", resp)
		}
		return nil, err
	}
	return response.Body, nil
}

// GetCoverArt returns a cover art image for a song, album, or artist.
//
// Optional Parameters:
//
//	size:            If specified, scale image to this size.
func (s *Client) GetCoverArt(id string, parameters map[string]string) (image.Image, error) {
	params := url.Values{}
	params.Add("id", id)
	if size, ok := parameters["size"]; ok {
		params.Add("size", size)
	}

	response, err := s.Request("GET", "getCoverArt", params)
	if err != nil {
		return nil, err
	}
	contentType := response.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/xml") || strings.HasPrefix(contentType, "application/xml") {
		// An error was returned
		responseBody, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		resp := Response{}
		err = xml.Unmarshal(responseBody, &resp)
		if err != nil {
			return nil, err
		}
		if resp.Error != nil {
			err = fmt.Errorf("Error #%d: %s\n", resp.Error.Code, resp.Error.Message)
		} else {
			err = fmt.Errorf("An error occurred: %#v\n", resp)
		}
		return nil, err
	}
	image, _, err := image.Decode(response.Body)
	if err != nil {
		return nil, err
	}
	return image, nil
}
func (s *Client) GetCoverArtUrl(id string) string {
	params := url.Values{}
	params.Add("id", id)
	return s.BaseUrl + "/rest/getCoverArt.view?" + params.Encode()
}

// GetAvatar returns the avatar (personal image) for a user.
func (s *Client) GetAvatar(username string) (image.Image, error) {
	params := url.Values{}
	params.Add("username", username)
	response, err := s.Request("GET", "getAvatar", params)
	if err != nil {
		return nil, err
	}
	contentType := response.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/xml") || strings.HasPrefix(contentType, "application/xml") {
		// An error was returned
		responseBody, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		resp := Response{}
		err = xml.Unmarshal(responseBody, &resp)
		if err != nil {
			return nil, err
		}
		if resp.Error != nil {
			err = fmt.Errorf("Error #%d: %s\n", resp.Error.Code, resp.Error.Message)
		} else {
			err = fmt.Errorf("An error occurred: %#v\n", resp)
		}
		return nil, err
	}
	image, _, err := image.Decode(response.Body)
	if err != nil {
		return nil, err
	}
	return image, nil
}

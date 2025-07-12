package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os/exec"
	"slices"
	"sort"
	"strconv"

	// "net/http"
	// "os"
	"regexp"
	"strings"
	"sync"
	"time"

	"ceombe/metadata"

	"github.com/F1ammetta/go-subsonic"
	"github.com/bwmarrin/discordgo"
	"github.com/go-tts/tts/pkg/speech"
	"github.com/lithammer/fuzzysearch/fuzzy"
	"layeh.com/gopus"
)

func handleLeaveCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	player.Queue = []string{}
	player.Stop = true
	player.Disconnect = true
	s.ChannelMessageSend(m.ChannelID, "Left voice channel.")
}

func handleTTSCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	text := strings.TrimPrefix(m.Content, config.Discord.Prefix+"tts ")

	user := m.Author

	voiceState, err := s.State.VoiceState(m.GuildID, user.ID)

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	if voiceState == nil {
		s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel to use this command.")
		return
	}

	voice, err := s.ChannelVoiceJoin(m.GuildID, voiceState.ChannelID, false, true)

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	audio, err := speech.FromText(text, speech.LangUs)

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	// bytesPerSecond := int64(48000 * 2 * 2) // SampleRate * Channels * BytesPerSample
	cmd := exec.Command("ffmpeg", "-re",
		"-i", "pipe:0",
		"-f", "s16le",
		"-ac", "2",
		"-ar", "48000",
		"-application", "lowdelay",
		"pipe:1")

	ffmpegIn, err := cmd.StdinPipe()
	ffmpegOut, err := cmd.StdoutPipe()
	ffmpegErr, err := cmd.StderrPipe()

	go func() {
		scanner := bufio.NewScanner(ffmpegErr)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	go func() {
		inbuf := make([]byte, 4096)

		for {
			n, err := audio.Read(inbuf)

			if err != nil {
				fmt.Println("Error: ", err)
				if err.Error() != "EOF" {
					break
				}
			}

			if n == 0 {
				break
			}

			_, err = ffmpegIn.Write(inbuf)

			if err != nil {
				fmt.Println("Error: ", err)
				if err.Error() != "EOF" {
					break
				}
			}
		}
		err = ffmpegIn.Close()

		if err != nil {
			fmt.Println("Error: ", err)
		}
	}()

	go func() {
		err := cmd.Start()

		encoder, err := gopus.NewEncoder(48000, 2, gopus.Audio)
		frameSize := 960
		maxBytes := 960 * 2 * 2

		if err != nil {
			fmt.Println("Error: ", err)
			if err.Error() != "EOF" {
				return
			}
		}

		reader := bufio.NewReader(ffmpegOut)

		for {
			if player.Paused || player.Skip {
				break
			}
			buffer := make([]byte, 4096)

			n, err := reader.Read(buffer)

			if err != nil {
				fmt.Println("Error: ", err)
				if err.Error() != "EOF" {
					break
				}
			}

			if n == 0 {
				break
			}

			encbuf := make([]int16, frameSize*2)

			for i := 0; i < maxBytes/2; i++ {
				encbuf[i] = int16(buffer[i*2]) | int16(buffer[i*2+1])<<8
			}

			opus, err := encoder.Encode(encbuf, frameSize, maxBytes)

			if err != nil {
				fmt.Println("Error: ", err)
				if err.Error() != "EOF" {
					break
				}
			}

			if voice.Ready == false || voice.OpusSend == nil {

				return
			}

			voice.OpusSend <- opus
		}
	}()

}

func getSongName(str string) string {
	var name string

	if after, ok := strings.CutPrefix(strings.TrimSpace(str), "id:"); ok {
		result, err := subsonicClient.GetSong(after)
		if err != nil {
			fmt.Println("Error: ", err)
			return ""
		}
		name = result.Artist + " - " + result.Title
	} else {
		result, err := subsonicClient.Search3(str, map[string]string{})
		if err != nil {
			fmt.Println("Error: ", err)
			return ""
		}

		name = result.Song[0].Artist + " - " + result.Song[0].Title
	}

	return name
}

func handleQueueCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	if len(player.Queue) == 0 {
		s.ChannelMessageSend(m.ChannelID, "Queue is empty.")
		return
	}

	var str strings.Builder

	str.WriteString("```")
	str.WriteString("Queue:\n")
	for i, song := range player.Queue {
		str.WriteString(fmt.Sprintf("%d. %s\n", i+1, getSongName(song)))
	}
	str.WriteString("```")

	s.ChannelMessageSend(m.ChannelID, str.String())
}

type YTDLPEntry struct {
	URL           string `json:"url"`
	Title         string `json:"title"`
	PlaylistTitle string `json:"playlist_title"`
}

func getPlaylistData(playlistURL string) (string, []string, error) {
	cmd := exec.Command("yt-dlp", "--flat-playlist", "--dump-json", playlistURL)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	var playlistTitle string
	var urls []string
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		var entry YTDLPEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			fmt.Printf("Error unmarshaling yt-dlp output line: %v\n", err)
			continue
		}

		if playlistTitle == "" {
			if entry.PlaylistTitle != "" {
				playlistTitle = entry.PlaylistTitle
			} else {
				playlistTitle = "YouTube Playlist" // Fallback if playlist_title is not available
			}
		}

		if entry.URL != "" {
			if !seen[entry.URL] {
				urls = append(urls, entry.URL)
				seen[entry.URL] = true
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("error reading yt-dlp stdout: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return "", nil, fmt.Errorf("yt-dlp command failed: %w", err)
	}

	return playlistTitle, urls, nil
}

func handleDownListCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	list_url := strings.TrimSpace(strings.TrimPrefix(m.Content, config.Discord.Prefix+"dl"))

	playlistTitle, list, err := getPlaylistData(list_url)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Error getting playlist data.")
		return
	}

	s.ChannelMessageSend(m.ChannelID, "Downloading Playlist: "+playlistTitle+" with "+strconv.Itoa(len(list))+" songs")

	var wg sync.WaitGroup
	var downloadedSongs []*metadata.Metadata
	var downloadErrors []error

	// Create channels to communicate between goroutines
	songsChan := make(chan *metadata.Metadata, len(list))
	errorsChan := make(chan error, len(list))

	// Now, download the non-duplicate songs fully
	for _, songURL := range list {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			cmd := exec.Command("python3", "song_dl.py", url)
			out, err := cmd.Output()
			if err != nil {
				errorsChan <- fmt.Errorf("error downloading song: %w", err)
				return
			}
			filePath := strings.TrimSpace(string(out))

			meta, err := metadata.GetMetadata(filePath)
			if err != nil {
				fmt.Println("AcoustID metadata failed, falling back to file tags:", err)
				meta, err = metadata.ReadTagsFromFile(filePath)
				if err != nil {
					fmt.Println("Failed to read tags from file, attempting to parse from filename:", err)
					// Fallback to parsing from filename
					fileName := strings.TrimSuffix(filePath, ".mp3") // Assuming .mp3 extension
					parts := strings.Split(fileName, " - ")
					if len(parts) >= 2 {
						meta = &metadata.Metadata{
							Title:  strings.TrimSpace(parts[0]),
							Artist: strings.TrimSpace(parts[1]),
							Album:  "Unknown Album", // Placeholder
						}
					} else {
						meta = &metadata.Metadata{
							Title:  fileName,
							Artist: "Unknown Artist",
							Album:  "Unknown Album",
						}
					}
				}
			}
			if meta != nil {
				songsChan <- meta
			} else {
				errorsChan <- fmt.Errorf("failed to get any metadata for song %s", url)
			}
		}(songURL)
	}

	wg.Wait()
	close(songsChan)
	close(errorsChan)

	for song := range songsChan {
		downloadedSongs = append(downloadedSongs, song)
	}

	for err := range errorsChan {
		downloadErrors = append(downloadErrors, err)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Downloaded %d songs", len(downloadedSongs)))

	if len(downloadErrors) > 0 {
		var errorMsgs []string
		for _, err := range downloadErrors {
			errorMsgs = append(errorMsgs, err.Error())
		}
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Errors during download: %s", strings.Join(errorMsgs, "; ")))
	}

	time.Sleep(50 * time.Second)

	// Get song IDs from subsonic
	var songIDs []string
	for _, song := range downloadedSongs {
		if song == nil {
			fmt.Println("Skipping nil song metadata.")
			continue
		}

		query := song.Artist + " " + song.Title

		result, err := subsonicClient.Search3(query, map[string]string{})
		if err != nil {
			fmt.Println("Error searching for song in Subsonic:", err)
			continue
		}
		if len(result.Song) > 0 {
			// Fuzzy find the best match from the search results
			songIDs = append(songIDs, result.Song[0].ID)
		} else {
			fmt.Printf("Downloaded song \"%s - %s\" but no results found in Subsonic for query \"%s\".\n", song.Artist, song.Title, query)
		}
	}

	// Create playlist
	if len(songIDs) > 0 {
		// Create the playlist with the first song
		err := subsonicClient.CreatePlaylist(map[string]string{
			"name":   playlistTitle,
			"songId": songIDs[0],
		})
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Error creating playlist.")
			return
		}

		// Get the new playlist's ID
		pls, err := subsonicClient.GetPlaylists(map[string]string{})
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "Could not retrieve playlists to find new playlist ID.")
			return
		}
		var newPlaylistID string
		for _, pl := range pls {
			if pl.Name == playlistTitle {
				newPlaylistID = pl.ID
				break
			}
		}

		// Add the rest of the songs
		if len(songIDs) > 1 {
			for _, songID := range songIDs[1:] {
				err := subsonicClient.UpdatePlaylist(newPlaylistID, map[string]string{
					"songIdToAdd": songID,
				})
				if err != nil {
					fmt.Printf("Error adding song %s to playlist %s: %s\n", songID, newPlaylistID, err)
				}
			}
		}

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Successfully created playlist '%s' with %d new songs.", playlistTitle, len(songIDs)))
	}

}
func handleShuffleCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	rand.Seed(time.Now().UnixNano())

	q := &player.Queue

	rand.Shuffle(len(*q), func(i, j int) {
		(*q)[i], (*q)[j] = (*q)[j], (*q)[i]
	})

	s.ChannelMessageSend(m.ChannelID, "Shuffled Queue.")
}

func handleDownCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	url := strings.TrimPrefix(m.Content, config.Discord.Prefix+"d")

	s.ChannelMessageSend(m.ChannelID, "Downloading song...")

	cmd := exec.Command("python3", "song_dl.py", url)
	out, err := cmd.Output()
	if err != nil {
		fmt.Println("Error downloading song:", err)
		s.ChannelMessageSend(m.ChannelID, "Error downloading song.")
		return
	}
	filePath := strings.TrimSpace(string(out))

	meta, err := metadata.GetMetadata(filePath)
	if err != nil {
		fmt.Println("Error getting metadata:", err)
		s.ChannelMessageSend(m.ChannelID, "Downloaded song, but failed to get metadata.")
		return
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Downloaded and tagged: %s - %s", meta.Artist, meta.Title))

}

func handleJoinCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	user := m.Author

	voiceState, err := s.State.VoiceState(m.GuildID, user.ID)

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	if voiceState == nil {
		s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel to use this command.")
		return
	}

	_, err = s.ChannelVoiceJoin(m.GuildID, voiceState.ChannelID, false, true)

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	s.ChannelMessageSend(m.ChannelID, "Joined voice channel.")
}

func handleSearchCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	fmt.Println("Search command")

	query := strings.TrimPrefix(m.Content, config.Discord.Prefix+"search")

	result, err := subsonicClient.Search3(query, map[string]string{})

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	if len(result.Song) == 0 {
		s.ChannelMessageSend(m.ChannelID, "No results found.")
		return
	}

	s.ChannelMessageSend(m.ChannelID, "Results: ")

	var str strings.Builder

	str.WriteString("```")
	str.WriteString("Artist - Title\n")
	for _, song := range result.Song {
		str.WriteString(fmt.Sprintf("%s - %s\n", song.Artist, song.Title))
	}
	str.WriteString("```")

	s.ChannelMessageSend(m.ChannelID, str.String())
}

func handleUploadCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	var errors []string

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Attempting to upload %d Files...", len(m.Attachments)))

	var wg sync.WaitGroup
	for _, file := range m.Attachments {
		if !strings.Contains(file.ContentType, "audio") {
			errors = append(errors, file.Filename)
			continue
		}

		// TODO: Spawn goroutine to download
		wg.Add(1)
		go downloadFile(file.ProxyURL, &wg)

	}

	wg.Wait()

	errstr := strings.Join(errors, "\n")
	var succ string
	if len(errors) == len(m.Attachments) {
		succ = "Failed to upload files, wrong format: Only audio files are allowed"
	} else {
		succ = "Upload Success"
	}
	var err string
	if len(errors) > 0 {
		err = fmt.Sprintf("```\n%s```", errstr)
	} else {
		err = ""
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s\n%s", succ, err))
}

func getId(stream_url string) (string, error) {

	if stream_url == "" {
		return "", errors.New("There is no song playing right now.")
	}

	urlRegex := regexp.MustCompile(`&id=([a-zA-Z0-9]*)&`)

	match := urlRegex.FindStringSubmatch(stream_url)

	if match == nil {
		return "", errors.New("Unexpected error ocurred getting song id.")
	}

	return match[1], nil
}
func handleAliasCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	alias := strings.TrimPrefix(m.Content, "~alias ")

	song_id, err := getId(player.Playing)

	song_id = "id:" + song_id

	if err != nil {
		s.ChannelMessageSend(m.ChannelID, err.Error())
	}

	for k := range aliases {
		if k == alias {
			s.ChannelMessageSend(m.ChannelID, "Alias already in database")
			return
		}
	}

	aliases[alias] = song_id

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Aliased %s to %s", alias, song_id))
}

func handleIdCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	stream_url := player.Playing

	song_id, err := getId(stream_url)

	if err != nil {
		s.ChannelMessageSend(m.ChannelID, err.Error())
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Now playing song id:%s", song_id))
}

func handlePlayListCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	res, err := subsonicClient.GetPlaylists(map[string]string{})

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	var titles []string

	for _, pl := range res {
		titles = append(titles, pl.Name)
	}

	a := fuzzy.RankFindFold(strings.TrimPrefix(m.Content, "~pl "), titles)

	sort.Sort(a)

	slices.Reverse(a)

	pl, err := subsonicClient.GetPlaylist(res[a[a.Len()-1].OriginalIndex].ID)
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Adding to queue Playlist: %s", pl.Name))

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	for _, song := range pl.Entry {
		player.Queue = append(player.Queue, fmt.Sprintf("id:%s", song.ID))
	}

	if player.Playing == "" {
		var newS *discordgo.MessageCreate = m
		newS.Content = config.Discord.Prefix + "play " + player.Queue[0]
		player.Queue = player.Queue[1:]
		player.Position = 0
		commandHandler(s, newS)
	}

}

func handlePlayCommand(s *discordgo.Session, m *discordgo.MessageCreate, prefix string) {
	player.Stop = false

	user := m.Author

	voiceState, err := s.State.VoiceState(m.GuildID, user.ID)

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	if voiceState == nil {
		s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel to use this command.")
		return
	}

	chann, err := s.ChannelVoiceJoin(m.GuildID, voiceState.ChannelID, false, true)

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	// s.ChannelMessageSend(m.ChannelID, "Joined voice channel.")

	query := strings.TrimPrefix(m.Content, config.Discord.Prefix+prefix)

	for alias, song_id := range aliases {
		if fuzzy.LevenshteinDistance(strings.TrimSpace(query), alias) < 4 {
			query = song_id
		}
	}

	var song *subsonic.Child
	if after, ok := strings.CutPrefix(strings.TrimSpace(query), "id:"); ok {
		result, err := subsonicClient.GetSong(after)
		if err != nil {
			fmt.Println("Error: ", err)
			return
		}
		song = result

	} else {
		result, err := subsonicClient.Search3(query, map[string]string{})
		if err != nil {
			fmt.Println("Error: ", err)
			return
		}

		if len(result.Song) == 0 {
			s.ChannelMessageSend(m.ChannelID, "No results found.")
			return
		}

		song = result.Song[0]
	}

	println("Playing: ", song.Artist, " - ", song.Title)
	println("Cover art: url", subsonicClient.GetCoverArtUrl(song.CoverArt, map[string]string{}))

	streamUrl, err := subsonicClient.GetStreamUrl(song.ID, map[string]string{"format": "opus", "maxBitRate": "128"})

	// stream, err := subsonicClient.Stream(song.ID, map[string]string{"format": "opus", "maxBitRate": "128"})

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	chann.Speaking(true)
	if player.Playing == "" {
		player.Playing = streamUrl
		player.Query = query

		_, error := s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Title:       "Now playing",
			Description: fmt.Sprintf("%s - %s", song.Artist, song.Title),
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: subsonicClient.GetCoverArtUrl(song.CoverArt, map[string]string{}),
			},
		})

		if error != nil {
			fmt.Println("Error: ", error)
			return
		}

		s.UpdateGameStatus(0, fmt.Sprintf("%s - %s", song.Artist, song.Title))

	} else {
		player.Queue = append(player.Queue, query)
		s.ChannelMessageSend(m.ChannelID, "Added to the queue.")
		println("Queue: ", player.Queue)

		_, error := s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Title:       "Added to queue",
			Description: fmt.Sprintf("%s - %s", song.Artist, song.Title),
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: subsonicClient.GetCoverArtUrl(song.CoverArt, map[string]string{}),
			},
		})

		if error != nil {
			fmt.Println("Error: ", error)
		}

		return
	}
	bytesPerSecond := int64(48000 * 2 * 2) // SampleRate * Channels * BytesPerSample
	seekTime := float64(player.Position) / float64(bytesPerSecond)
	cmd := exec.Command("ffmpeg", "-re",
		"-ss", fmt.Sprintf("%.2f", seekTime),
		"-i", player.Playing,
		"-f", "s16le",
		"-ac", "2",
		"-ar", "48000",
		"-application", "lowdelay",
		"pipe:1")

	ffmpegOut, err := cmd.StdoutPipe()
	ffmpegErr, err := cmd.StderrPipe()

	go func() {
		scanner := bufio.NewScanner(ffmpegErr)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	go func() {
		err := cmd.Start()

		encoder, err := gopus.NewEncoder(48000, 2, gopus.Audio)
		frameSize := 960
		maxBytes := 960 * 2 * 2

		if err != nil {
			fmt.Println("Error: ", err)
			if err.Error() != "EOF" {
				return
			}
		}

		reader := bufio.NewReader(ffmpegOut)
		var position int64 = player.Position

		for {
			if player.Paused || player.Skip || player.Stop {
				break
			}
			buffer := make([]byte, 4096)

			n, err := reader.Read(buffer)

			if err != nil {
				fmt.Println("Error: ", err)
				if err.Error() != "EOF" {
					break
				}
			}

			if n == 0 {
				break
			}

			encbuf := make([]int16, frameSize*2)

			for i := 0; i < maxBytes/2; i++ {
				encbuf[i] = int16(buffer[i*2]) | int16(buffer[i*2+1])<<8
			}

			opus, err := encoder.Encode(encbuf, frameSize, maxBytes)

			if err != nil {
				fmt.Println("Error: ", err)
				if err.Error() != "EOF" {
					break
				}
			}

			if chann.Ready == false || chann.OpusSend == nil {

				return
			}

			chann.OpusSend <- opus
			position += int64(n)
		}
		player.Playing = ""

		player.Skip = false
		println("Song finished.", player.Skip)
		s.UpdateGameStatus(0, "")
		if player.Paused {
			player.Position = position
		} else if len(player.Queue) > 0 {
			player.Query = ""
			s.ChannelMessageSend(m.ChannelID, "Playing next song.")
			println("Queue: ", player.Queue)
			var newS *discordgo.MessageCreate = m
			newS.Content = config.Discord.Prefix + "play " + player.Queue[0]
			player.Queue = player.Queue[1:]
			player.Position = 0
			commandHandler(s, newS)

		} else if player.Loop {
			player.Query = ""
			var newS *discordgo.MessageCreate = m
			newS.Content = config.Discord.Prefix + "play " + query
			player.Position = 0
			s.ChannelMessageSend(m.ChannelID, "Looping song.")
			commandHandler(s, newS)

		} else {
			player.Query = ""
			player.Position = 0
			player.Stop = false
		}

		if player.Disconnect {
			chann.Disconnect()
			player.Disconnect = false
		}

	}()

}

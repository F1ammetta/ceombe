package main

import (
	"bufio"
	"fmt"
	"slices"
	"sort"
	"sync"

	// "net/http"
	// "os"
	"os/exec"
	"strings"

	// "ceombe/go-subsonic"

	"github.com/F1ammetta/go-subsonic"
	"github.com/bwmarrin/discordgo"
	"github.com/go-tts/tts/pkg/speech"
	"github.com/lithammer/fuzzysearch/fuzzy"

	// "github.com/pelletier/go-toml/v2"
	"layeh.com/gopus"
)

func handleLeaveCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
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

	player.Queue = []string{}

	player.Stop = true

	println(len(player.Queue))

	_, err = s.ChannelVoiceJoin(m.GuildID, "", false, true)

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

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

func handleDownCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	url := strings.TrimPrefix(m.Content, config.Discord.Prefix+"d")

	s.ChannelMessageSend(m.ChannelID, "Downloading song...")

	cmd := exec.Command("python3", "song_dl.py", url)

	stdout, err := cmd.StdoutPipe()

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	err = cmd.Run()

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	s.ChannelMessageSend(m.ChannelID, "Downloaded song.")

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

	}()

}

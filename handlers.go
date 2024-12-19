package main

import (
	"bufio"
	"fmt"
	// "net/http"
	// "os"
	"os/exec"
	"strings"

	// "ceombe/go-subsonic"

	"github.com/bwmarrin/discordgo"
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

	_, err = s.ChannelVoiceJoin(m.GuildID, "", false, true)

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	s.ChannelMessageSend(m.ChannelID, "Left voice channel.")
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
		str.WriteString(fmt.Sprintf("%d. %s\n", i+1, song))
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

	result, err := subsonicClient.Search3(query, map[string]string{})
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	if len(result.Song) == 0 {
		s.ChannelMessageSend(m.ChannelID, "No results found.")
		return
	}

	song := result.Song[0]

	println("Playing: ", song.Artist, " - ", song.Title)
	println("Cover art: url", subsonicClient.GetCoverArtUrl(song.CoverArt))

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
				URL: subsonicClient.GetCoverArtUrl(song.CoverArt),
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
				URL: subsonicClient.GetCoverArtUrl(song.CoverArt),
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
			newS.Content = config.Discord.Prefix + "play" + player.Queue[0]
			player.Queue = player.Queue[1:]
			player.Position = 0
			commandHandler(s, newS)

		} else if player.Loop {
			player.Query = ""
			var newS *discordgo.MessageCreate = m
			newS.Content = config.Discord.Prefix + "play" + query
			player.Position = 0
			s.ChannelMessageSend(m.ChannelID, "Looping song.")
			commandHandler(s, newS)

		} else {
			player.Query = ""
			player.Position = 0
		}

	}()

}

package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/delucks/go-subsonic"
	"github.com/pelletier/go-toml/v2"
	"net/http"
	"os"
	"strings"
)

type Server struct {
	Url      string
	Username string
	Password string
}

type Discord struct {
	Token  string
	Prefix string
}

type Config struct {
	Server  Server
	Discord Discord
}

var config Config
var subsonicClient subsonic.Client

func commandHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, config.Discord.Prefix) {
		return
	}

	println("Command: ", m.Content)

	if m.Content == config.Discord.Prefix+"ping" {
		s.ChannelMessageSend(m.ChannelID, "Pong!")
	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"search") {
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

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"play") {
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

		s.ChannelMessageSend(m.ChannelID, "Joined voice channel.")

		query := strings.TrimPrefix(m.Content, config.Discord.Prefix+"play")

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

		stream, err := subsonicClient.Stream(song.ID, map[string]string{"format": "opus", "maxBitRate": "128"})

		if err != nil {
			fmt.Println("Error: ", err)
			return
		}

		chann.Speaking(true)

		for {
			buffer := make([]byte, 320)
			n, err := stream.Read(buffer)

			if err != nil {
				fmt.Println("Error: ", err)
				break
			}

			if n == 0 {
				break
			}

			// fmt.Println("buffer: ", buffer)

			chann.OpusSend <- buffer

		}

	} else if m.Content == config.Discord.Prefix+"join" {
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

}

func main() {
	config.loadToml()

	discord, err := connectToDiscord()

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	subsonicClient, err = connectToSubsonic()

	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	defer discord.Close()

	discord.AddHandler(commandHandler)

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	<-make(chan struct{})
}

func connectToSubsonic() (subsonic.Client, error) {
	httpClient := &http.Client{}

	subsonicClient := subsonic.Client{
		Client:       httpClient,
		BaseUrl:      config.Server.Url,
		User:         config.Server.Username,
		ClientName:   "Ceombe",
		PasswordAuth: true,
	}

	err := subsonicClient.Authenticate(config.Server.Password)

	if err != nil {
		return subsonic.Client{}, err
	}

	return subsonicClient, nil
}

func connectToDiscord() (*discordgo.Session, error) {

	discord, err := discordgo.New("Bot " + config.Discord.Token)

	if err != nil {
		return nil, err
	}

	err = discord.Open()

	if err != nil {
		return nil, err
	}

	return discord, nil
}

func (c *Config) loadToml() {
	file, err := os.ReadFile("config.toml")

	if err != nil {
		fmt.Println("Error: ", err)
	}

	err = toml.Unmarshal(file, c)

	if err != nil {
		fmt.Println("Error: ", err)
	}

}

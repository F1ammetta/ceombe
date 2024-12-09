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

		str.WriteString("Artist | Title\n")
		str.WriteString("```")
		for _, song := range result.Song {
			str.WriteString(fmt.Sprintf("%s - %s\n", song.Artist, song.Title))
		}
		str.WriteString("```")

		s.ChannelMessageSend(m.ChannelID, str.String())

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

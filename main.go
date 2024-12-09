package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	// "github.com/delucks/go-subsonic"
	// "net/http"
	"github.com/pelletier/go-toml/v2"
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

func commandHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, config.Discord.Prefix) {
		return
	}

	if m.Content == config.Discord.Prefix+"ping" {
		s.ChannelMessageSend(m.ChannelID, "Pong!")
	}
}

func main() {
	config.loadToml()

	discord, err := discordgo.New("Bot " + config.Discord.Token)

	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return
	}

	discord.AddHandler(commandHandler)

	err = discord.Open()

	if err != nil {
		fmt.Println("Error opening connection: ", err)
		return
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	<-make(chan struct{})
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

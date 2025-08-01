package main

import (
	// "bufio"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"sync"

	// "os/exec"
	"strings"
	"time"

	"github.com/F1ammetta/go-subsonic"

	"github.com/bwmarrin/discordgo"
	"github.com/pelletier/go-toml/v2"
	// "layeh.com/gopus"
)

type Server struct {
	Url       string
	Username  string
	Password  string
	Music_dir string
}

type Discord struct {
	Token  string
	Prefix string
}

type Config struct {
	Server  Server
	Discord Discord
}
type Player struct {
	Playing    string   // Path to the current song
	Query      string   // Query for the current song
	Queue      []string // Queue of songs to play
	Position   int64    // Current playback position in bytes
	Paused     bool     // Indicates whether playback is paused
	Stop       bool     // Stops playback
	Loop       bool     // Channel to signal resume
	Skip       bool     // Indicates whether to skip the current song
	Disconnect bool     // Disconnects from voice channel
}

var config Config
var subsonicClient subsonic.Client
var player Player
var aliases map[string]string

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

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"search ") {

		handleSearchCommand(s, m)

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"play ") {

		handlePlayCommand(s, m, "play")

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"p ") {

		handlePlayCommand(s, m, "p")

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"pl ") {

		handlePlayListCommand(s, m)

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"id") {

		handleIdCommand(s, m)

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"upload") {

		handleUploadCommand(s, m)

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"dl ") {

		handleDownListCommand(s, m)
	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"shuffle") {

		handleShuffleCommand(s, m)

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"alias ") {

		handleAliasCommand(s, m)

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"d ") {

		handleDownCommand(s, m)

	} else if strings.HasPrefix(m.Content, config.Discord.Prefix+"tts ") {

		handleTTSCommand(s, m)

	} else if m.Content == config.Discord.Prefix+"queue" || m.Content == config.Discord.Prefix+"q" {

		handleQueueCommand(s, m)

	} else if m.Content == config.Discord.Prefix+"leave" {

		handleLeaveCommand(s, m)

	} else if m.Content == config.Discord.Prefix+"clear" {

		player.Queue = []string{}

	} else if m.Content == config.Discord.Prefix+"stop" {

		player.Queue = []string{}
		player.Stop = true

	} else if m.Content == config.Discord.Prefix+"join" {

		handleJoinCommand(s, m)

	} else if m.Content == config.Discord.Prefix+"skip" || m.Content == config.Discord.Prefix+"s" {

		player.Skip = true
		s.ChannelMessageSend(m.ChannelID, "Skipping song.")

	} else if m.Content == config.Discord.Prefix+"pause" {

		player.Paused = true
		s.ChannelMessageSend(m.ChannelID, "Paused.")

	} else if m.Content == config.Discord.Prefix+"resume" {

		player.Paused = false
		s.ChannelMessageSend(m.ChannelID, "Resumed.")
		var newS *discordgo.MessageCreate = m
		newS.Content = config.Discord.Prefix + "play " + player.Query //for now at least
		commandHandler(s, newS)

	} else if m.Content == config.Discord.Prefix+"loop" {

		player.Loop = !player.Loop

		switch player.Loop {
		case true:
			s.ChannelMessageSend(m.ChannelID, "Looping enabled.")
		case false:
			s.ChannelMessageSend(m.ChannelID, "Looping disabled.")
		}
	}

}

func checkForExit(d *discordgo.Session) {
	for {

		// wait for 3 seconds
		<-time.After(3 * time.Second)

		v := d.VoiceConnections

		if len(v) == 0 {
			continue
		}

		for _, c := range v {

			guild, err := d.State.Guild(c.GuildID)

			if err != nil {
				fmt.Println("Error: ", err)
				continue
			}

			if guild == nil {
				continue
			}

			if len(guild.VoiceStates) == 1 {
				_, err = d.ChannelVoiceJoin(guild.ID, "", false, true)

				if err != nil {
					fmt.Println("Error: ", err)
					continue
				}

				fmt.Println("Disconnected.")
			}
		}

	}
}

func getFilename(resp *http.Response) string {
	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		_, params, err := mime.ParseMediaType(cd)
		if err == nil && params["filename"] != "" {
			return params["filename"]
		}
	}

	return path.Base(resp.Request.URL.Path)
}

func downloadFile(url string, wg *sync.WaitGroup) {
	defer wg.Done()

	res, err := http.Get(url)

	if err != nil {
		return
	}

	filename := config.Server.Music_dir + "/" + getFilename(res)

	out, err := os.Create(filename)
	if err != nil {
		return
	}
	defer out.Close()

	_, err = io.Copy(out, res.Body)
	if err != nil {
		return
	}
}

func main() {
	config.loadToml()
	handleAliases()

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

	player.Stop = false

	discord.AddHandler(commandHandler)

	fmt.Println("Bot is now running. Press CTRL-C to exit.")

	go checkForExit(discord)

	<-make(chan struct{})
}

func connectToSubsonic() (subsonic.Client, error) {
	httpClient := &http.Client{}

	subsonicClient := subsonic.Client{
		Client:       httpClient,
		BaseUrl:      config.Server.Url,
		User:         config.Server.Username,
		ClientName:   "Ceombe",
		PasswordAuth: false,
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

func updateAliases() {

	for {

		bytes, err := toml.Marshal(aliases)

		if err != nil {
			fmt.Println("Error: ", err)
		}

		os.WriteFile("aliases.toml", bytes, 0666)

		if err != nil {
			fmt.Println("Error: ", err)
		}

		// wait for 30 seconds
		<-time.After(30 * time.Second)

	}
}

func handleAliases() {
	file, err := os.ReadFile("aliases.toml")

	if err != nil {
		fmt.Println("Error: ", err)
	}

	err = toml.Unmarshal(file, &aliases)

	if err != nil {
		fmt.Println("Error: ", err)
	}

	go updateAliases()
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

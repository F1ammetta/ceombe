package main

import (
	"bufio"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/delucks/go-subsonic"
	"github.com/pelletier/go-toml/v2"
	"layeh.com/gopus"
	"net/http"
	"os"
	"os/exec"
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

		cmd := exec.Command("ffmpeg", "-re",
			"-i", "-",
			"-f", "s16le",
			"-ac", "2",
			"-ar", "48000",
			"-b:a", "128k",
			"-application", "lowdelay",
			"pipe:1")

		ffmpegIn, err := cmd.StdinPipe()
		ffmpegOut, err := cmd.StdoutPipe()

		go func() {
			err := cmd.Start()

			encoder, err := gopus.NewEncoder(48000, 2, gopus.Audio)
			frameSize := 960
			maxBytes := 960 * 2 * 2

			if err != nil {
				fmt.Println("Error: ", err)
				return
			}

			reader := bufio.NewReader(ffmpegOut)

			for {
				buffer := make([]byte, 4096)

				n, err := reader.Read(buffer)

				if err != nil {
					fmt.Println("Error: ", err)
					break
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
					break
				}

				if chann.Ready == false || chann.OpusSend == nil {

					return
				}

				println("Sending opus packet of size: ", len(opus))

				chann.OpusSend <- opus
			}

		}()

		for {
			streambuffer := make([]byte, 4096)

			n, err := stream.Read(streambuffer)

			if err != nil {
				fmt.Println("get stream Error: ", err)
				break
			}

			if n == 0 {
				break
			}

			ffmpegIn.Write(streambuffer)
		}

		// for {
		// 	buffer := make([]byte, 320)
		// 	n, err := stream.Read(buffer)
		//
		// 	if err != nil {
		// 		fmt.Println("Error: ", err)
		// 		break
		// 	}
		//
		// 	if n == 0 {
		// 		break
		// 	}
		//
		// 	// fmt.Println("buffer: ", buffer)
		//
		// 	chann.OpusSend <- buffer
		//
		// }

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

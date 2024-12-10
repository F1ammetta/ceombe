module ceombe

go 1.23.4

replace ceombe/go-subsonic => ./go-subsonic

require (
	// github.com/delucks/go-subsonic v0.0.0-20240806025900-2a743ec36238
	ceombe/go-subsonic v0.0.0
	github.com/bwmarrin/discordgo v0.28.1
	github.com/pelletier/go-toml/v2 v2.2.3
	layeh.com/gopus v0.0.0-20210501142526-1ee02d434e32
)

require (
	github.com/gorilla/websocket v1.4.2 // indirect
	golang.org/x/crypto v0.0.0-20210421170649-83a5a9bb288b // indirect
	golang.org/x/sys v0.0.0-20201119102817-f84b799fce68 // indirect
)

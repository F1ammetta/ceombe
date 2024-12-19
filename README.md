# Ceombe

Ceombe is a music playing discord bot that can play music from any subsonic compatible server.

If you have it running on the subsonic server, the bot can download music and add it to the server for you to listen to.

## Installation

To install the bot, you need to have python 3.6 or higher and Go 1.13 or higher installed on your system.

just clone the repository and run the following command to install the dependencies:

```bash
pip install -r requirements.txt
go mod tidy
```

## Usage

To run the bot, you need to have a discord bot token and a subsonic server url, username and password.

You can get a discord bot token by creating a new bot on the discord developer portal.

You can find an example of the configuration file in the `example-config.toml` file.

Same goes for the `example-auth.toml` file. This one is for spotify integration to add metadata to the songs that are downloaded.

After you have created the configuration files, rename them to `config.toml` and `auth.toml` respectively.

To run the bot, just build using the following command:

```bash
go build
```

And then run the the binary in the same directory as the configuration files, and the python script.

```bash
./ceombe
```











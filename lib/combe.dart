import 'package:nyxx/nyxx.dart';
import 'package:subsonic_api/subsonic_api.dart';
import 'package:toml/toml.dart';

void addCommand(NyxxGateway client, Config config, String command,
    Future<void> Function(MessageCreateEvent) callback) {
  client.onMessageCreate.listen((event) async {
    if (event.message.content == '${config.commandPrefix}$command') {
      await callback(event);
    }
  });
}

class Player {}

class Config {
  String discordToken;
  String commandPrefix;
  String subsonicUrl;
  String subsonicUser;
  String subsonicSalt;
  String subsonicToken;

  Config(this.discordToken, this.commandPrefix, this.subsonicUrl,
      this.subsonicUser, this.subsonicSalt, this.subsonicToken);

  factory Config.fromToml(Map<String, dynamic> toml) {
    Config config = Config('', '', '', '', '', '');
    config.discordToken = toml['discord']['token'];
    config.commandPrefix = toml['discord']['prefix'];
    config.subsonicUrl = toml['server']['url'];
    config.subsonicUser = toml['server']['username'];
    config.subsonicSalt = createSalt();
    config.subsonicToken =
        createToken(toml['server']['password'], config.subsonicSalt);
    return config;
  }

  @override
  String toString() {
    return 'Config(token: $discordToken, prefix: $commandPrefix, subsonicUrl: $subsonicUrl, subsonicUser: $subsonicUser, subsonicSalt: $subsonicSalt, subsonicToken: $subsonicToken)';
  }
}

import 'package:nyxx/nyxx.dart';
import 'package:subsonic_api/subsonic_api.dart';
import 'package:toml/toml.dart';

class Config {
  String discordToken;
  String subsonicUrl;
  String subsonicUser;
  String subsonicSalt;
  String subsonicToken;

  Config(this.discordToken, this.subsonicUrl, this.subsonicUser,
      this.subsonicSalt, this.subsonicToken);

  factory Config.fromToml(Map<String, dynamic> toml) {
    Config config = Config('', '', '', '', '');
    config.discordToken = toml['general']['token'];
    config.subsonicUrl = toml['server']['url'];
    config.subsonicUser = toml['server']['username'];
    config.subsonicSalt = createSalt();
    config.subsonicToken =
        createToken(toml['server']['password'], config.subsonicSalt);
    return config;
  }

  @override
  String toString() {
    return 'Config(token: $discordToken, subsonicUrl: $subsonicUrl, subsonicUser: $subsonicUser, subsonicSalt: $subsonicSalt, subsonicToken: $subsonicToken)';
  }
}

import 'package:combe/combe.dart' as combe;
import 'package:nyxx/nyxx.dart';
import 'package:subsonic_api/subsonic_api.dart';
import 'package:toml/toml.dart';

final String configFile = 'config.toml';

Future<void> main() async {
  final toml = await TomlDocument.load(configFile);
  final config = combe.Config.fromToml(toml.toMap());

  final client = await Nyxx.connectGateway(
      config.discordToken, GatewayIntents.allUnprivileged);

  final user = await client.users.fetchCurrentUser();

  final subsonic = SubSonicClient(config.subsonicUrl, config.subsonicUser,
      config.subsonicToken, config.subsonicSalt, 'ceombe', '1.16.1');
}

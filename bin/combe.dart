import 'package:combe/combe.dart';
import 'package:nyxx/nyxx.dart';
import 'package:nyxx_lavalink/nyxx_lavalink.dart';
import 'package:subsonic_api/subsonic_api.dart';
import 'package:toml/toml.dart';

final String configFile = 'config.toml';

Future<void> main() async {
  final toml = await TomlDocument.load(configFile);
  final config = Config.fromToml(toml.toMap());

  final client =
      await Nyxx.connectGateway(config.discordToken, GatewayIntents.all);

  final subsonic = SubSonicClient(config.subsonicUrl, config.subsonicUser,
      config.subsonicToken, config.subsonicSalt, 'ceombe', '1.16.1');

  addCommand(client, config, 'join', (event) async {
    // get the voice channel the user is in
    final voiceChannel = client
        .guilds[event.guildId!].voiceStates[event.message.author.id]!.channel;

    // join the voice channel
    client.updateVoiceState(
        event.guildId!,
        GatewayVoiceStateBuilder(
            channelId: voiceChannel!.id, isMuted: false, isDeafened: true));
  });

  client.onEvent.listen((event) {
    if (event is VoiceServerUpdateEvent) {
      final endpoint = event.endpoint;
      final token = event.token;
      final guildId = event.guildId;

      // connect to the websocet and start streaming audio
    }

    if (event is VoiceStateUpdateEvent) {
      // check if joined the voice channel successfully
    }
  });
}

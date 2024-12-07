import 'dart:io';

import 'package:combe/combe.dart';
import 'package:nyxx/nyxx.dart';
import 'package:subsonic_api/subsonic_api.dart';
import 'package:toml/toml.dart';

final String configFile = 'config.toml';

Future<void> main() async {
  final toml = await TomlDocument.load(configFile);
  final config = Config.fromToml(toml.toMap());

  final client =
      await Nyxx.connectGateway(config.discordToken, GatewayIntents.all);

  final user = await client.users.fetchCurrentUser();

  final subsonic = SubSonicClient(config.subsonicUrl, config.subsonicUser,
      config.subsonicToken, config.subsonicSalt, 'ceombe', '1.16.1');

  addCommand(client, config, 'join', (event) async {
    // get the voice channel the user is in
    final voiceChannel = client
        .guilds[event.guildId!].voiceStates[event.message.author.id]?.channel;

    // join the voice channel
    if (voiceChannel == null) {
      await event.message.channel.sendMessage(MessageBuilder(
          content: 'You are not in a voice channel',
          referencedMessage:
              MessageReferenceBuilder.reply(messageId: event.message.id)));
      return;
    }

    client.updateVoiceState(
        event.guildId!,
        GatewayVoiceStateBuilder(
            channelId: voiceChannel.id, isMuted: false, isDeafened: true));
  });

  addCommand(client, config, 'leave', (event) async {
    // leave the voice channel
    client.updateVoiceState(
        event.guildId!,
        GatewayVoiceStateBuilder(
            channelId: null, isMuted: false, isDeafened: true));
  });

  late String sessionId = '';
  client.onEvent.listen((event) {
    if (event is VoiceStateUpdateEvent) {
      print('Voice state update: ${event.state.guildId}');
      sessionId = event.state.sessionId;
    } else if (event is VoiceServerUpdateEvent) {
      final endpoint = event.endpoint;
      final token = event.token;
      final guildId = event.guildId;

      print('Voice server update: $endpoint, $token, $guildId');
      print('Session id: $sessionId');

      // connect to the websocet and start streaming audio
      if (endpoint != null) {
        connectToVoiceChannel(
            config, user, sessionId, guildId, endpoint, token);
      }
    }

    // Should work but crashes when leaving voice channel
  });
}

import 'dart:io';
import 'package:nyxx/nyxx.dart';
import 'package:subsonic_api/subsonic_api.dart';
import 'package:toml/toml.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';
import 'package:synchronized/synchronized.dart';
import 'package:cryptography/cryptography.dart';
import 'package:crypto/crypto.dart';
import 'package:pointycastle/pointycastle.dart';

void addCommand(NyxxGateway client, Config config, String command,
    Future<void> Function(MessageCreateEvent) callback) {
  client.onMessageCreate.listen((event) async {
    if (event.message.content == '${config.commandPrefix}$command') {
      await callback(event);
    }
  });
}

List<int> secretKey = [];

void connectToVoiceChannel(Config config, User user, String sessionId,
    Snowflake guildId, String endpoint, String token) {
  final ws = WebSocketChannel.connect(Uri.parse('wss://$endpoint/?v=4'));

  ws.sink.add(jsonEncode({
    'op': 0,
    'd': {
      'server_id': guildId.toString(),
      'user_id': user.id.toString(),
      'session_id': sessionId,
      'token': token,
    }
  }));

  ws.stream.listen((event) {
    event = JsonDecoder().convert(event);
    print(event);

    if (event['op'] == 2) {
      final ip = event['d']['ip'];
      final port = event['d']['port'];
      final ssrc = event['d']['ssrc'];
      final mode = event['d']['modes'][0];

      ws.sink.add(jsonEncode({
        'op': 5,
        'd': {
          'speaking': 1,
          'delay': 0,
          'ssrc': ssrc,
        }
      }));
      connectUdp(config, ip, port, ssrc, ws);
      ws.sink.add(jsonEncode({
        'op': 1,
        'd': {
          'protocol': 'udp',
          'data': {
            'address': config.discordUdpIp,
            'port': config.discordUdpPort,
            'mode': mode,
          }
        }
      }));
    } else if (event['op'] == 8) {
      final interval = event['d']['heartbeat_interval'];
      sendHearbeat(ws, interval);
    } else if (event['op'] == 4) {
      final list = event['d']['secret_key'];
      for (var i = 0; i < list.length; i++) {
        secretKey.add(list[i]);
      }
    }
  }, onDone: () {
    print('Connection closed');
  }, onError: (error) {
    print('Error: $error');
  });
}

Future<void> connectUdp(
    Config config, String ip, int port, int ssrc, WebSocketChannel ws) async {
  final udp = await RawDatagramSocket.bind(
      InternetAddress("192.168.1.73"), config.discordUdpPort);

  final testfile = 'test.flac';
  var sequenceNumber = 0;
  var timestamp = 0;

  final ffmpeg = await Process.start('ffmpeg', [
    '-re',
    '-i',
    testfile,
    '-ac',
    '2',
    '-ar',
    '48000',
    '-f',
    'opus',
    '-b:a',
    '96k',
    '-application',
    'lowdelay',
    'pipe:1'
  ]);

  final audioBuffer = BytesBuilder();

  ffmpeg.stdout.listen((data) async {
    audioBuffer.add(data);

    while (audioBuffer.length >= 960) {
      final opusData = audioBuffer.takeBytes().sublist(0, 960);

      final rtpHeader = generateRtpHeader(
          sequenceNumber: sequenceNumber, timestamp: timestamp, ssrc: ssrc);

      sequenceNumber = (sequenceNumber + 1) % 65536;
      timestamp += 960; // 20ms at 48kHz

      final encryptedPacket = await encryptPacket(
        Uint8List.fromList(rtpHeader),
        Uint8List.fromList(opusData),
        secretKey,
      );

      udp.send(encryptedPacket, InternetAddress(ip), port);
    }
  });

  udp.listen((event) {
    if (event == RawSocketEvent.read) {
      final datagram = udp.receive();
      if (datagram != null) {
        // Handle any UDP responses if needed
        print('Received ${datagram.data}');
      }
    }
  });
}

// Example RTP Header Generation
Uint8List generateRtpHeader(
    {required int sequenceNumber, required int timestamp, required int ssrc}) {
  final header = BytesBuilder();

  // First byte: Version (2 bits), Padding (1 bit), Extension (1 bit), CSRC count (4 bits)
  header.addByte(0x80); // Version 2, no padding, no extension, no CSRC

  // Second byte: Marker (1 bit), Payload Type (7 bits)
  // Opus is usually payload type 120
  header.addByte(0x78); // Marker 0, Payload Type 120

  // Sequence Number (16 bits)
  header.addByte((sequenceNumber >> 8) & 0xFF);
  header.addByte(sequenceNumber & 0xFF);

  // Timestamp (32 bits)
  for (int i = 3; i >= 0; i--) {
    header.addByte((timestamp >> (i * 8)) & 0xFF);
  }

  // SSRC (32 bits)
  for (int i = 3; i >= 0; i--) {
    header.addByte((ssrc >> (i * 8)) & 0xFF);
  }

  return header.toBytes();
}

Future<Uint8List> encryptPacket(
    Uint8List rtpHeader, Uint8List opusData, List<int> secretKey) async {
  // Create AES-GCM algorithm
  final algorithm = AesGcm.with256bits();

  // Use first 12 bytes of RTP header as nonce
  final nonce = rtpHeader.sublist(0, 12);

  // Combine RTP header and Opus data
  final plaintext = Uint8List.fromList([...rtpHeader, ...opusData]);

  // Perform encryption
  final secretKeyData = SecretKey(secretKey);
  final encrypted = await algorithm.encrypt(
    plaintext,
    secretKey: secretKeyData,
    nonce: nonce,
  );

  // Combine ciphertext and MAC
  final encryptedPacket =
      Uint8List.fromList([...encrypted.cipherText, ...encrypted.mac.bytes]);

  return encryptedPacket;
}

Future<void> sendHearbeat(WebSocketChannel ws, dynamic interval) async {
  while (true) {
    await Future.delayed(Duration(milliseconds: interval.round()));
    final payload =
        jsonEncode({"op": 3, "d": DateTime.now().millisecondsSinceEpoch});
    ws.sink.add(payload);
    print('Sent heartbeat: $payload');
  }
}

class Player {}

class Config {
  String discordToken;
  String discordUdpIp;
  int discordUdpPort;
  String commandPrefix;
  String subsonicUrl;
  String subsonicUser;
  String subsonicSalt;
  String subsonicToken;

  Config(
      this.discordToken,
      this.discordUdpIp,
      this.discordUdpPort,
      this.commandPrefix,
      this.subsonicUrl,
      this.subsonicUser,
      this.subsonicSalt,
      this.subsonicToken);

  factory Config.fromToml(Map<String, dynamic> toml) {
    Config config = Config('', '', 0, '', '', '', '', '');
    config.discordToken = toml['discord']['token'];
    config.discordUdpIp = toml['discord']['bot_ip'];
    config.discordUdpPort = toml['discord']['bot_port'];
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
    return 'Config{discordToken: $discordToken, discord_upd_ip: $discordUdpIp, discord_upd_port: $discordUdpPort, commandPrefix: $commandPrefix, subsonicUrl: $subsonicUrl, subsonicUser: $subsonicUser, subsonicSalt: $subsonicSalt, subsonicToken: $subsonicToken}';
  }
}

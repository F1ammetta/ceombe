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
      print('Connected to voice channel with encryption mode: $mode');
      connectUdp(mode, config, ip, port, ssrc, ws);
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

Uint8List int32BigEndianBytes(int value) =>
    Uint8List(4)..buffer.asByteData().setInt32(0, value, Endian.big);

Uint8List int16BigEndianBytes(int value) =>
    Uint8List(2)..buffer.asByteData().setInt16(0, value, Endian.big);

Future<void> connectUdp(String mode, Config config, String ip, int port,
    int ssrc, WebSocketChannel ws) async {
  final udp = await RawDatagramSocket.bind(InternetAddress.anyIPv4, 0);
  final ffmpegudp =
      await RawDatagramSocket.bind(InternetAddress("127.0.0.1"), 5004);

  final testfile = 'test.opus';
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
    '128k',
    '-application',
    'lowdelay',
    '-acodec',
    'libopus',
    '-frame_duration',
    '20',
    '-payload_size',
    '960',
    'rtp://127.0.0.1:5004'
  ]);

  var audioBuffer = BytesBuilder();

  udp.listen((event) {
    if (event == RawSocketEvent.read) {
      final datagram = udp.receive();
      if (datagram != null) {
        // Handle any UDP responses if needed
        parseResponse(ws, mode, datagram.data);
      }
    }
  });
  final pingPacket = createRequestPacket(ssrc);
  udp.send(pingPacket, InternetAddress(ip), port);

  while (secretKey.isEmpty) {
    await Future.delayed(Duration(milliseconds: 100));
  }
  print('Secret key: $secretKey');
  ffmpegudp.listen((event) async {
    if (event == RawSocketEvent.read) {
      final datagram = ffmpegudp.receive();
      if (datagram != null) {
        audioBuffer.add(datagram.data);
        var opusData = Uint8List(0);
        while (audioBuffer.length >= 960) {
          opusData = audioBuffer.takeBytes().sublist(0, 960);
        }
        if (opusData.isEmpty) {
          return;
        }
        final rtpHeader = generateRtpHeader(
            sequenceNumber: sequenceNumber, timestamp: timestamp, ssrc: ssrc);
        final encryptedPacket =
            await encryptPacket(rtpHeader, opusData, secretKey);

        print('Sending packet with sequence number $sequenceNumber');
        udp.send(encryptedPacket, InternetAddress(ip), port);
        sequenceNumber++;
        timestamp += 960;
      }
    }
  });
}

Uint8List createRequestPacket(int ssrc) {
  final packet = Uint8List(70 + 4); // Total size: 2 + 2 + 4 + 64 = 70
  final buffer = ByteData.sublistView(packet);

  // Set Type (0x01 for request)
  buffer.setUint16(0, 0x01); // 2 bytes

  // Set Length (70)
  buffer.setUint16(2, 70); // 2 bytes

  // Set SSRC
  buffer.setUint32(4, ssrc); // 4 bytes

  // Padding (automatically 0 since Uint8List initializes with 0)

  return packet;
}

void parseResponse(WebSocketChannel ws, String mode, Uint8List response) {
  final buffer = ByteData.sublistView(response);

  // Validate Type
  final type = buffer.getUint16(0);
  if (type != 0x02) {
    // TODO: handdle responses
    // print(response);
    return;
  }

  // Validate Length
  final length = buffer.getUint16(2);
  if (length != 70) {
    print('Invalid response length: $length');
    return;
  }

  // Extract SSRC
  final ssrc = buffer.getUint32(4);
  print('SSRC: $ssrc');

  // Extract Address
  final addressBytes = response.sublist(8, 72); // Null-terminated string
  final ip = String.fromCharCodes(addressBytes.takeWhile((byte) => byte != 0));
  print('External IP: $ip');

  // Extract Port
  final port = buffer.getUint16(72);
  print('External Port: $port');

  ws.sink.add(jsonEncode({
    'op': 1,
    'd': {
      'protocol': 'udp',
      'data': {
        'address': ip,
        'port': port,
        'mode': mode,
      }
    }
  }));
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
  final sequenceNumberBytes = int16BigEndianBytes(sequenceNumber);
  for (int i = 0; i < 2; i++) {
    header.addByte(sequenceNumberBytes[i]);
  }

  // Timestamp (32 bits)
  final timestampBytes = int32BigEndianBytes(timestamp);
  for (int i = 0; i < 4; i++) {
    header.addByte(timestampBytes[i]);
  }

  // SSRC (32 bits)
  final ssrcBytes = int32BigEndianBytes(ssrc);
  for (int i = 0; i < 4; i++) {
    header.addByte(ssrcBytes[i]);
  }

  print('Generated RTP Header: ${header.toBytes()}');
  return header.toBytes();
}

int nonce = 0;

Future<Uint8List> encryptPacket(
    Uint8List rtpHeader, Uint8List opusData, List<int> secretKey) async {
  final aesGcm = AesGcm.with256bits();
  final noncebytes = int32BigEndianBytes(nonce++);

  final secretKeyData = SecretKey(secretKey);

  final encrypted = await aesGcm.encrypt(
    opusData,
    secretKey: secretKeyData,
    nonce: noncebytes,
  );

  return Uint8List.fromList([...rtpHeader, ...encrypted.cipherText]);
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
    return 'Config{discordToken: $discordToken, commandPrefix: $commandPrefix, subsonicUrl: $subsonicUrl, subsonicUser: $subsonicUser, subsonicSalt: $subsonicSalt, subsonicToken: $subsonicToken}';
  }
}

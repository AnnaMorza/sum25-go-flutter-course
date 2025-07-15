import 'package:flutter/material.dart';
import 'dart:async';
import '../services/websocket_service.dart';

class WebSocketScreen extends StatefulWidget {
  const WebSocketScreen({super.key});

  @override
  State<WebSocketScreen> createState() => _WebSocketScreenState();
}

class _WebSocketScreenState extends State<WebSocketScreen> {
  final WebSocketService _webSocketService = WebSocketService();
  final TextEditingController _messageController = TextEditingController();
  final TextEditingController _userIdController = TextEditingController();
  final TextEditingController _delayController = TextEditingController();
  final List<WebSocketMessage> _messages = [];

  bool _isConnected = false;
  bool _isConnecting = false;

  late StreamSubscription<WebSocketMessage> _messageSubscription;
  late StreamSubscription<bool> _connectionSubscription;

  @override
  void initState() {
    super.initState();
    _userIdController.text = 'user_${DateTime.now().millisecondsSinceEpoch % 10000}';
    _delayController.text = '0';

    _messageSubscription = _webSocketService.messages.listen((message) {
      if (!mounted) return;
      setState(() {
        _messages.add(message);
        if (_messages.length > 20) _messages.removeAt(0);
      });
    });

    _connectionSubscription = _webSocketService.connectionStatus.listen((connected) {
      if (!mounted) return;
      setState(() {
        _isConnected = connected;
        _isConnecting = false;
      });
    });
  }

  @override
  void dispose() {
    _messageSubscription.cancel();
    _connectionSubscription.cancel();
    _webSocketService.dispose();
    _messageController.dispose();
    _userIdController.dispose();
    _delayController.dispose();
    super.dispose();
  }

  Future<void> _connect() async {
    if (_userIdController.text.trim().isEmpty) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Please enter a user ID')),
        );
      }
      return;
    }
    setState(() => _isConnecting = true);
    await _webSocketService.connect('ws://localhost:8081/ws',
        userId: _userIdController.text.trim());
    if (!mounted || !_isConnected) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Failed to connect to WebSocket server')),
      );
    }
  }

  Future<void> _disconnect() async {
    await _webSocketService.disconnect();
  }

  void _sendMessage() {
    final text = _messageController.text.trim();
    if (text.isEmpty || !_isConnected) return;
    final int delay = int.tryParse(_delayController.text) ?? 0;
    if (delay > 0) {
      _webSocketService.sendDelayedMessage(text, delay);
    } else {
      _webSocketService.sendMessage(text);
    }
    _messageController.clear();
  }

  void _sendPing() {
    if (_isConnected) _webSocketService.sendPing();
  }

  String _formatTimestamp(DateTime t) =>
      '${t.hour.toString().padLeft(2, '0')}:${t.minute.toString().padLeft(2, '0')}:${t.second.toString().padLeft(2, '0')}';

  Color _getColor(String type) {
    switch (type) {
      case 'system':
        return Colors.blue;
      case 'notification':
        return Colors.orange;
      case 'pong':
        return Colors.green;
      default:
        return Colors.black87;
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('WebSocket Chat'),
        backgroundColor: Theme.of(context).colorScheme.inversePrimary,
        actions: [
          IconButton(
            key: const Key('pingButton'),
            icon: Icon(_isConnected ? Icons.cloud_done : Icons.cloud_off),
            onPressed: _sendPing,
          ),
        ],
      ),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              children: [
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        key: const Key('userIdField'),
                        controller: _userIdController,
                        decoration: const InputDecoration(
                          labelText: 'User ID',
                          border: OutlineInputBorder(),
                        ),
                        enabled: !_isConnected,
                      ),
                    ),
                    const SizedBox(width: 8),
                    ElevatedButton(
                      key: const Key('connectButton'),
                      onPressed: _isConnecting
                          ? null
                          : (_isConnected ? _disconnect : _connect),
                      child: _isConnecting
                          ? const SizedBox(
                              width: 16,
                              height: 16,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : Text(_isConnected ? 'Disconnect' : 'Connect'),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                Row(
                  children: [
                    Icon(
                      _isConnected ? Icons.circle : Icons.circle_outlined,
                      color: _isConnected ? Colors.green : Colors.red,
                      size: 16,
                    ),
                    const SizedBox(width: 8),
                    Text(
                      _isConnected ? 'Connected' : 'Not connected',
                      style: TextStyle(
                        color: _isConnected ? Colors.green : Colors.red,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
          Expanded(
            child: Container(
              child: _messages.isEmpty
                  ? const Center(
                      child: Text('No messages yet.\nConnect and start chatting!'),
                    )
                  : ListView.builder(
                      key: const Key('messageList'),
                      padding: const EdgeInsets.all(8),
                      itemCount: _messages.length,
                      itemBuilder: (context, i) {
                        final msg = _messages[i];
                        final isOwn = msg.user == _webSocketService.currentUserId;
                        return Align(
                          alignment:
                              isOwn ? Alignment.centerRight : Alignment.centerLeft,
                          child: Container(
                            margin: const EdgeInsets.symmetric(vertical: 2),
                            padding: const EdgeInsets.all(8),
                            decoration: BoxDecoration(
                              color: isOwn ? Colors.blue[100] : Colors.grey[200],
                              borderRadius: BorderRadius.circular(8),
                            ),
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                if (!isOwn || msg.type != 'message')
                                  Text(
                                    msg.user,
                                    style: TextStyle(
                                      fontWeight: FontWeight.bold,
                                      color: _getColor(msg.type),
                                    ),
                                  ),
                                Text(
                                  msg.content,
                                  style: TextStyle(
                                    color: _getColor(msg.type),
                                  ),
                                ),
                                Text(
                                  _formatTimestamp(msg.timestamp),
                                  style: const TextStyle(
                                    fontSize: 10,
                                    color: Colors.grey,
                                  ),
                                ),
                                if (msg.delay != null && msg.delay! > 0)
                                  Text(
                                    ' (${msg.delay}ms)',
                                    style: const TextStyle(
                                      fontSize: 10,
                                      color: Colors.orange,
                                    ),
                                  ),
                              ],
                            ),
                          ),
                        );
                      },
                    ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              children: [
                Row(
                  children: [
                    const Text('Delay (ms): '),
                    const SizedBox(width: 8),
                    SizedBox(
                      width: 60,
                      child: TextField(
                        key: const Key('delayField'),
                        controller: _delayController,
                        keyboardType: TextInputType.number,
                        decoration: const InputDecoration(
                          border: OutlineInputBorder(),
                          isDense: true,
                        ),
                      ),
                    ),
                    const SizedBox(width: 16),
                    ElevatedButton.icon(
                      key: const Key('pingActionButton'),
                      onPressed: _isConnected ? _sendPing : null,
                      icon: const Icon(Icons.network_ping),
                      label: const Text('Ping'),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                Row(
                  children: [
                    Expanded(
                      child: TextField(
                        key: const Key('messageField'),
                        controller: _messageController,
                        decoration: const InputDecoration(
                          hintText: 'Type a message...',
                          border: OutlineInputBorder(),
                        ),
                        onSubmitted: (_) => _sendMessage(),
                        enabled: _isConnected,
                      ),
                    ),
                    const SizedBox(width: 8),
                    ElevatedButton(
                      key: const Key('sendButton'),
                      onPressed: _isConnected ? _sendMessage : null,
                      child: const Text('Send'),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

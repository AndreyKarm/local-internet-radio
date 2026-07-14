# Local Radio

A lightweight, self-hosted radio server designed to stream music directly from your local filesystem.

## Features

### Audio & Metadata

* **Broad Format Support:** Stream MP3, FLAC, and WAV files.
* **Rich Metadata:** Automatic extraction of Title, Artist, Album, Track Number, and Duration.
* **Continuous Playback:** A robust streaming engine designed for uninterrupted listening.

### Management & Interface

* **Web Dashboard:** A dedicated web interface for real-time station management.
* **Integrated Uploads:** Manage your library directly via the browser.
* **Remote Control:** Stream and monitor your station from any device on your network.

### Streaming Capabilities

* **Standardized Output:** High-quality MP3 stream compatible with most network audio players.
* **Low Latency:** Optimized for real-time playback in applications and games.

## Getting Started

### Installation via Docker

1. **Deploy the Stack**  
  Run the following command to start the radio server and the `rustfs` storage backend:

 ```bash
 docker-compose up -d
 ```

2. **Access the Interface**

 Open your web browser and navigate to: [http://127.0.0.1:3000](http://127.0.0.1:3000)

3. **Populate your Library**

 Use the web interface to upload your music files.

4. **Start Listening**

  Once files are uploaded, your local radio station is live.

## Technical Integration

For applications, games, or hardware that support MP3 network streaming, use the following endpoint:

Stream URL: [http://127.0.0.1:8080/stream](http://127.0.0.1:8080/stream)

## Compatibility

The following applications and platforms have been verified for use with this service:

### Media Players

  -VLC Media Player

### Games

  -War Thunder

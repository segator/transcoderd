# TranscoderD

TranscoderD is a solution that helps to reduce of your video library by converting it to efficient video codec like h265.

TranscoderD have a server and a worker application. The server will manage and schedule the jobs, the worker will do the actual encoding.
You can have as many workers you want, locally or remote in multiple machines.

## Features
- Convert video files to h265
- Convert audio to AAC for better compatibility.
- Remove unwanted/duplicated audio tracks
- PGS (image like subtitles) to SRT: Plex and emby likes to burn this subtitle format to video, meaning almost guaranteed you are going to transcode the video when playing, this is why we convert it to SRT


## Prerequisites
- Docker
- Postgres database
- Grafana (Optional): For statistics and monitoring 


## Server Installation

### Config file transcoderd.yml

Replace it by your own values, remember to create manually the DB scheme and PG user.
```yaml
database:
  host: 192.168.1.55
  user: db_user
  password: db_pass
  scheme: encode

web:
  token: my_secret_token # replace it by your own secret

scheduler:
  sourcePath: /mnt/media
  deleteOnComplete: false # if true, the original file will be deleted after job is completed
  minFileSize: 100 # minimum file size to be considered for encoding
```

### Run
```bash
docker run -d \
       --name transcoder-daemon \
       --restart always \
       -p 8080:8080 \
       -v /mnt/media:/mnt/media 
      -v ./transcoderd.yml:/etc/transcoderd/config.yml \
      ghcr.io/segator/transcoderd:latest-server
```

### Grafana Statistics
- Add postgres as datasource for grafana.
- Import this [grafana-dashboard](./grafana-dashboard.json) and change the data source to your own Postgres database.


## Worker Installation
### Run
```bash
docker run -d \
       --name transcoderd-worker \
       --restart=always \
       -v /tmp:/tmp \ # Ensure to have enough space (+50G, but depends on your biggest media size) on your temporal folder, as the worker will use it heavily for encoding
       --hostname $(hostname) \ 
       ghcr.io/segator/transcoderd:latest-worker \
       --web.token my_secret_token \ # Replace it for the same value as in the server config
       --web.domain http://192.168.1.55:8080 # Replace it for the server IP or public endpoint if you want remote access.
        
```
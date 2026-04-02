An RTMP server
Using this as a ref https://github.com/melpon/rfc/blob/master/rtmp.md

ffmpeg -v trace -re -i assets/sample.mp4 -c copy -f flv rtmp://localhost:8080/

FROM golang:1.4-onbuild
MAINTAINER Chris Winters "chris@cwinters.com"

RUN mkdir -p /app/scripts
WORKDIR /app/scripts

# Default is to run a korra session with your directory of scripts mounted to /app/scripts
CMD ["sessions", "-dir", "/app/scripts"]

# Entrypoint provides for you to run reports and other commands -- the 
# golang-onbuild container renames the binary to 'app'
ENTRYPOINT ["app"]

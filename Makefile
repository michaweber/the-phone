BUILD_DATE=`date -u +%Y%m%d%H%M%S`
BRANCH=`git rev-parse --abbrev-ref HEAD`
VERSION=`git describe --tags`

BUILDFLAGS = --ldflags "-X github.com/michaweber/thephone/config.Version=$(VERSION) -X github.com/michaweber/thephone/config.Env=$(BRANCH) -X github.com/michaweber/thephone/config.Build=$(BUILD_DATE)"
build:
	env GOOS=linux GOARCH=arm GOARM=6 go build -v -o bin/thephone $(BUILDFLAGS) 

copy: 
	scp bin/thephone home-phone:thephone

deploy: build service-stop copy service-start
	echo "Done"

copy-sounds: 
	ssh home-phone "rm -r sounds/; mkdir -p sounds"
	scp -r sounds/ home-phone:

test:
	ssh home-phone -t './thephone'

install-service:
	cat systemd/thephone.service | ssh home-phone "sudo tee -a /lib/systemd/system/thephone.service"
	ssh home-phone "sudo chmod 644 /lib/systemd/system/thephone.service"
	ssh home-phone "sudo systemctl daemon-reload"
	ssh home-phone "sudo systemctl enable thephone.service"
	ssh home-phone "sudo systemctl start thephone.service"
	ssh home-phone "sudo systemctl status thephone.service"

service-status:
	ssh home-phone "sudo systemctl status thephone.service"

service-stop:
	ssh home-phone "sudo systemctl stop thephone.service"

service-start:
	ssh home-phone "sudo systemctl start thephone.service"

service-restart: service-stop service-start
	echo "Service restarted"

service-log:
	ssh home-phone "sudo journalctl -f -u thephone.service"

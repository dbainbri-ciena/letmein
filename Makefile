latest:
	docker build -t cord/letmein:latest .

build:
	docker build -t cord/letmein:$(shell date +"%Y%m%dT%H%M") .

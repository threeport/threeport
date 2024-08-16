.DEFAULT_GOAL := help
CURRENTTAG:=$(shell git describe --tags --abbrev=0)
NEWTAG ?= $(shell bash -c 'read -p "Please provide a new tag (currnet tag - ${CURRENTTAG}): " newtag; echo $$newtag')

#help: @ List available tasks
help:
	@clear
	@echo "Usage: make COMMAND"
	@echo "Commands :"
	@grep -E '[a-zA-Z\.\-]+:.*?@ .*$$' $(MAKEFILE_LIST)| tr -d '#' | awk 'BEGIN {FS = ":.*?@ "}; {printf "\033[32m%-7s\033[0m - %s\n", $$1, $$2}'

#deps: @ Install dependencies
deps:
	@pip install mkdocs mkdocs-material mkdocs-material-extensions mike  

#run: @ Run mkdocs
run:
	@mkdocs serve 2>&1 > /dev/null

#stop: @ Stop mkdocs
stop:
	@ps aux | grep 'python.*mkdocs[[:space:]]serve' | awk {'print $$2'} | xargs kill -9

#release: @ Create and push a new tag
release:
	$(eval NT=$(NEWTAG))
	@echo -n "Are you sure to create and push ${NT} tag? [y/N] " && read ans && [ $${ans:-N} = y ]
	@git tag -a -m "Cut ${NT} release" ${NT}
	@git push origin ${NT}
	@git push
	@echo "Done."

#version: @ Print current version(tag)
version:
	@echo $(shell git describe --tags --abbrev=0)

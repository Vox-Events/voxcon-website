build: $(shell find src i18n -type f)
	cd .make; go run build.go

.PHONY: watch
watch:
	cd .make; go run build.go --watch
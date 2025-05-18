build: $(shell find src i18n -type f) ## build this website
	@cd .make; go run build.go

.PHONY: watch
watch: ## same as build, but watch for file changes
	@cd .make; go run build.go --watch
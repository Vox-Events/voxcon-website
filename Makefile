SHELL:=/bin/bash
.DEFAULT_GOAL := help

##
### Main targets
##
include .make/build.mk
include .make/clean.mk

include .make/help.mk

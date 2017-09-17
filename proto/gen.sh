#!/bin/bash
protoc --go_out=plugins=grpc:gen calc.proto

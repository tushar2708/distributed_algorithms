#!/bin/bash

# sudo apt install protobuf-compiler

pwd

ls -ltrh proto/raft_messages.proto

# Define variables
PROTO_DIR="./proto/"
OUT_DIR="./grpc/"

# Create output directory if it does not exist
mkdir -p "$OUT_DIR"

# Compile the proto files
protoc --go_out="$OUT_DIR" \
       --go-grpc_out="$OUT_DIR" \
       --proto_path="$PROTO_DIR" \
       "$PROTO_DIR"/*.proto

# Notify the user
echo "Proto files compiled and output to $OUT_DIR"

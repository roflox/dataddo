# Use the official Go image as a base image
FROM golang:latest

# Set the working directory inside the container
WORKDIR /app

# Copy the local package files to the container's workspace
COPY . .

# Build the Go application
RUN go build -o myapi ./main

# Expose a port (if your Go program listens on a specific port)
EXPOSE 8080

# Command to run the executable
# optional parameter db_file which is file path
CMD ["./myapi", "-debug=false"]
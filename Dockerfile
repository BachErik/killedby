# Use an official Golang runtime as a parent image
FROM golang:1.23.2-alpine

# Set the working directory inside the container
WORKDIR /app

# Set environment variables
ENV GITHUB_USERNAME=bacherik
ENV GITHUB_REPOSITORY=killedby.json

# Copy the go.mod and go.sum files into the working directory
COPY go.mod go.sum ./

# Install dependencies
RUN go mod download

# Copy the entire project and build it
COPY . .

# Build the Go app
RUN go build -o /killedby

# Expose port 8080 to the outside world
EXPOSE 8080

# Command to run the executable
CMD ["/killedby"]

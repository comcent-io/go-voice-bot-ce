# Use the official Golang image as a build stage
FROM golang:1.25.0 AS build

# Set the working directory inside the container
WORKDIR /app

# Copy the go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code to the container
COPY . .

# Build the Go app
RUN go build -o main .

# Use a minimal base image to reduce the size of the final image
FROM golang:1.25.0

# Set the working directory inside the container
WORKDIR /app

# Copy the binary from the build stage
COPY --from=build /app/main .

# Expose the port the app runs on
EXPOSE 5080

# Command to run the binary
CMD ["./main"]

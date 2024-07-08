# killedby

killedby is a project that collects information about discontinued projects, products, and services. It fetches data from a GitHub repository and displays it in a web interface. The project can be set up using Docker and supports fetching data from a specified GitHub repository.

## Demo

A live demo of this project is hosted at [killedby.bacherik.de](http://killedby.bacherik.de).

## Features

- Fetches and displays information about discontinued projects from a GitHub repository.
- Group projects by year and company.
- Maintains the aspect ratio of logos in the header and project cards.
- Easily configurable via environment variables.

## Setup

### Prerequisites

- Docker
- Git

### Step-by-Step Setup

#### 1. **Make your own config files**
Make your own repo from the template [Repository](https://github.com/BachErik/killedby.json)
#### 2. **Start as Docker container**
```bash
docker run -d -p 8080:8080 -e GITHUB_USERNAME=yourusername -e GITHUB_REPOSITORY=killedby.json bacherik/killedby:latest
```
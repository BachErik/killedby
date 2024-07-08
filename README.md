# Killed by

Killed by is an engaging web application designed to showcase discontinued projects, products, and services, providing insights into their lifecycle and impact. Utilizing data from a specific GitHub repository, this application presents the information in an accessible web interface, emphasizing user-friendly navigation and detailed project breakdowns.

## Features

- **Data-Driven Insights**: Automatically fetches and displays comprehensive details about discontinued projects, including their initiation and termination dates.
- **Dynamic Grouping**: Organizes projects by year and company for easy browsing.
- **Responsive Design**: Ensures that logos and project cards maintain their aspect ratios and are displayed correctly across devices.
- **Customization**: Easily configured through environment variables to suit different deployment needs.

## Live Demo

You can view a live demo of the application at: [killedby.bacherik.de](http://killedby.bacherik.de)

## Getting Started

### Prerequisites

Before setting up the project, ensure you have Docker and Git installed on your system. These tools are necessary for running the application in a containerized environment.

### Installation

1. **Clone the Repository**:
   Begin by cloning the template repository to create your own project setup:
   ```bash
   git clone https://github.com/BachErik/killedby.json your-project-name
   cd your-project-name
   ```

2. **Docker Setup**:
   Build and run the Docker container:
   ```bash
   docker build -t yourusername/killedby .
   docker run -d -p 8080:8080 -e GITHUB_USERNAME=yourusername -e GITHUB_REPOSITORY=your-repo.json yourusername/killedby
   ```

## Usage

Once the application is running, access it by navigating to `http://localhost:8080` in your web browser. Explore the discontinued projects by company or year, and gain insights into various trends and historical data points across different industries.

## Configuration

Modify the following environment variables as needed to point to different data sources or customize the application behavior:
- `GITHUB_USERNAME`: Username of the GitHub account where the data repository is located.
- `GITHUB_REPOSITORY`: Name of the repository containing JSON files with project data.

## Contributing

Contributions are welcome! If you have suggestions for improvements or bug fixes, please fork the repository and submit a pull request.

## License

This project is open-sourced under the MIT License. See the LICENSE file for more details.
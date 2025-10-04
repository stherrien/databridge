# DataBridge

A powerful flow-based data processing platform inspired by Apache NiFi, built with Go and Next.js.

## Overview

DataBridge is a visual data integration platform that enables users to design, control, and monitor data flows through a web-based interface. It provides a drag-and-drop canvas for building data pipelines with real-time monitoring and provenance tracking.

## Features

- **Visual Flow Designer**: Drag-and-drop interface for building data pipelines
- **Real-time Monitoring**: Live metrics and status updates for all components
- **Provenance Tracking**: Complete lineage tracking for all data flowing through the system
- **Template System**: Reusable flow templates with variable substitution
- **Processor Library**: Extensible collection of data processors for various operations
- **REST API**: Comprehensive API for programmatic control and integration
- **Settings Management**: Configurable system settings across multiple categories

## Architecture

### Backend (Go)
- High-performance REST API server
- Badger DB for data persistence
- Concurrent flow execution engine
- Real-time metrics collection

### Frontend (Next.js)
- Modern React-based UI with TypeScript
- Real-time updates via Server-Sent Events
- Responsive design with Tailwind CSS
- Interactive flow canvas

## Quick Start

### Prerequisites
- Go 1.21 or higher
- Node.js 18 or higher
- Make

### Installation

1. Clone the repository:
```bash
git clone https://github.com/stherrien/databridge.git
cd databridge
```

2. Install dependencies:
```bash
# Install Go dependencies
go mod download

# Install frontend dependencies
cd frontend
npm install
cd ..
```

### Running the Application

**Development Mode:**

Terminal 1 - Start the backend:
```bash
make dev
```

Terminal 2 - Start the frontend:
```bash
cd frontend
npm run dev
```

Access the application at `http://localhost:3000`

**Production Build:**
```bash
make build
./build/databridge
```

## API Endpoints

### Flows
- `GET /api/flows` - List all flows
- `POST /api/flows` - Create a new flow
- `GET /api/flows/{id}` - Get flow details
- `PUT /api/flows/{id}` - Update a flow
- `DELETE /api/flows/{id}` - Delete a flow
- `POST /api/flows/{id}/start` - Start a flow
- `POST /api/flows/{id}/stop` - Stop a flow

### Processors
- `GET /api/processors` - List all processors
- `POST /api/processors` - Create a processor
- `GET /api/processors/{id}` - Get processor details
- `PUT /api/processors/{id}` - Update a processor
- `DELETE /api/processors/{id}` - Delete a processor

### Connections
- `GET /api/connections` - List all connections
- `POST /api/connections` - Create a connection
- `DELETE /api/connections/{id}` - Delete a connection

### Monitoring
- `GET /api/system/status` - System status
- `GET /api/system/health` - Health check
- `GET /api/system/metrics` - System metrics
- `GET /api/monitoring/stream` - SSE stream for real-time updates

### Templates
- `GET /api/templates` - List all templates
- `POST /api/templates` - Create a template
- `POST /api/templates/{id}/instantiate` - Instantiate a template

### Provenance
- `GET /api/provenance/events` - List provenance events
- `GET /api/provenance/lineage/{flowFileId}` - Get FlowFile lineage
- `GET /api/provenance/stats` - Get provenance statistics

### Settings
- `GET /api/settings` - Get all settings
- `PUT /api/settings` - Update settings
- `POST /api/settings/reset` - Reset to defaults

## Project Structure

```
databridge/
├── cmd/
│   └── databridge/         # Main application entry point
├── internal/
│   ├── api/                # REST API handlers
│   ├── core/               # Core business logic
│   ├── processor/          # Processor implementations
│   └── storage/            # Data persistence layer
├── frontend/
│   ├── app/                # Next.js pages and layouts
│   ├── components/         # React components
│   ├── lib/                # Utilities and API client
│   └── public/             # Static assets
├── data/                   # Runtime data (gitignored)
├── Makefile                # Build and development tasks
└── go.mod                  # Go module definition
```

## Available Processors

- **GetFile** - Read files from filesystem
- **PutFile** - Write files to filesystem
- **GenerateFlowFile** - Generate test data
- **UpdateAttribute** - Modify FlowFile attributes
- **TransformJSON** - Transform JSON data
- **ExecuteSQL** - Execute SQL queries
- **InvokeHTTP** - Make HTTP requests
- **RouteOnAttribute** - Route based on attributes
- **CompressContent** - Compress/decompress content
- **LogAttribute** - Log FlowFile attributes

## Configuration

Configuration is managed through the Settings API and can be modified via the web UI or API endpoints.

### Key Configuration Areas:
- **General**: System name, concurrency, logging
- **Flow**: FlowFile settings, scheduling
- **Storage**: Data retention, persistence options
- **Monitoring**: Metrics collection, alerting
- **Security**: Authentication, authorization (coming soon)

## Development

### Building
```bash
make build      # Build backend binary
make clean      # Clean build artifacts
```

### Testing
```bash
make test       # Run Go tests
cd frontend && npm test  # Run frontend tests
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Inspired by Apache NiFi's flow-based programming model
- Built with Go, Next.js, and modern web technologies

# DataBridge Frontend Test Plan & Coverage Report

## Test Date: 2025-10-03
## Testing Method: Manual walkthrough with documentation
## Backend Status: Running on port 8080

---

## 1. Frontend Architecture Overview

### Pages (Next.js App Router)
- `/` - Home/Dashboard
- `/flows` - Flows list page
- `/flows/[id]` - Flow canvas/editor
- `/monitoring` - Monitoring dashboard
- `/processors` - Processor library
- `/provenance` - Provenance tracking
- `/templates` - Flow templates
- `/settings` - Application settings

### Key Components
- **Flow Canvas**: ReactFlow-based visual flow editor
- **Component Palette**: Drag-drop processor library
- **Property Panel**: Processor configuration
- **Toolbar**: Flow controls (save, run, stop)
- **Monitoring**: Real-time metrics and charts
- **Provenance**: Event timeline and lineage

### State Management
- SWR for data fetching
- React hooks for local state
- Custom hooks for flow operations

### API Integration
- Axios-based HTTP client
- Base URL: http://localhost:8080/api
- Endpoints: flows, processors, connections, monitoring, provenance

---

## 2. Test Scenarios

### 2.1 Navigation & Routing
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] Navigate to home page
- [ ] Access flows list
- [ ] Navigate to individual flow
- [ ] Access monitoring dashboard
- [ ] Access processors page
- [ ] Access provenance page
- [ ] Access templates page
- [ ] Access settings page
- [ ] Verify sidebar navigation
- [ ] Test back/forward browser navigation

**Expected Behavior**:
- All routes should load without errors
- Active route highlighted in sidebar
- Proper page titles and headers

---

### 2.2 Flows List Page (`/flows`)
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] View list of flows
- [ ] Create new flow
- [ ] Delete existing flow
- [ ] Search/filter flows
- [ ] Sort flows by name/date
- [ ] Click flow to navigate to editor

**API Endpoints Used**:
- GET `/api/flows` - List flows
- POST `/api/flows` - Create flow
- DELETE `/api/flows/{id}` - Delete flow

**Expected Data Structure**:
```json
{
  "flows": [
    {"id": "uuid", "name": "Flow Name"}
  ],
  "total": 1
}
```

---

### 2.3 Flow Canvas (`/flows/[id]`)
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] Load flow with processors
- [ ] Load flow with connections
- [ ] Display empty flow
- [ ] Handle loading state
- [ ] Handle error state (flow not found)
- [ ] Zoom in/out
- [ ] Pan canvas
- [ ] Fit view to content

**API Endpoints Used**:
- GET `/api/flows/{id}` - Get flow details

**Expected Data Structure**:
```json
{
  "id": "uuid",
  "name": "Flow Name",
  "processors": [
    {
      "id": "uuid",
      "type": "GenerateFlowFile",
      "config": {
        "name": "Processor Name",
        "properties": {}
      },
      "position": {"x": 0, "y": 0}
    }
  ],
  "connections": [
    {
      "id": "uuid",
      "sourceId": "uuid",
      "targetId": "uuid",
      "relationship": "success"
    }
  ]
}
```

---

### 2.4 Component Palette
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] Display available processor types
- [ ] Search/filter processors
- [ ] Drag processor to canvas
- [ ] Show processor descriptions
- [ ] Group processors by category

**API Endpoints Used**:
- GET `/api/processors/types` - List processor types

**Expected Behavior**:
- Processors should be draggable
- Drop should create processor at cursor position
- New processor should appear on canvas

---

### 2.5 Processor Operations
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] Add processor to canvas
- [ ] Select processor
- [ ] Move processor
- [ ] Configure processor properties
- [ ] Delete processor
- [ ] Duplicate processor
- [ ] Copy/paste processor

**API Endpoints Used**:
- POST `/api/flows/{flowId}/processors` - Create processor
- PUT `/api/flows/{flowId}/processors/{id}` - Update processor
- DELETE `/api/flows/{flowId}/processors/{id}` - Delete processor

**Expected Behavior**:
- Processor appears immediately on canvas
- Property panel opens on selection
- Changes persist to backend
- Processor removed on delete

---

### 2.6 Connection Operations
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] Create connection between processors
- [ ] Select connection
- [ ] Delete connection
- [ ] View connection relationship
- [ ] Handle invalid connections
- [ ] Display connection labels

**API Endpoints Used**:
- POST `/api/flows/{flowId}/connections` - Create connection
- DELETE `/api/flows/{flowId}/connections/{id}` - Delete connection

**Expected Behavior**:
- Connection created by dragging from output to input
- Connection line visible between processors
- Connection deleted on selection + delete
- Invalid connections rejected

---

### 2.7 Property Panel
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] Display processor properties
- [ ] Edit text properties
- [ ] Edit numeric properties
- [ ] Edit boolean properties
- [ ] Edit select/dropdown properties
- [ ] Validate required fields
- [ ] Save property changes
- [ ] Cancel property changes
- [ ] Handle sensitive properties

**Expected Behavior**:
- Properties load from processor metadata
- Changes update backend on save
- Validation errors displayed
- Panel closeable

---

### 2.8 Toolbar Operations
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] Save flow
- [ ] Start flow
- [ ] Stop flow
- [ ] Toggle component palette
- [ ] Toggle property panel
- [ ] View flow name
- [ ] Export flow
- [ ] Import flow

**API Endpoints Used**:
- POST `/api/flows/{flowId}/start` - Start flow
- POST `/api/flows/{flowId}/stop` - Stop flow

**Expected Behavior**:
- Save persists changes
- Start button starts processors
- Stop button stops processors
- Toggles show/hide panels

---

### 2.9 Monitoring Dashboard
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] View system metrics
- [ ] View processor metrics
- [ ] View throughput charts
- [ ] View queue depth charts
- [ ] View live event feed
- [ ] Refresh metrics
- [ ] Filter metrics by time range

**API Endpoints Used**:
- GET `/api/monitoring/metrics` - System metrics
- GET `/api/monitoring/processors` - Processor metrics
- GET `/api/monitoring/events` - Live events

**Expected Behavior**:
- Charts display real-time data
- Metrics update automatically
- Event feed shows recent activity

---

### 2.10 Provenance Tracking
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] View provenance timeline
- [ ] Search provenance events
- [ ] View event details
- [ ] View lineage graph
- [ ] Filter by event type
- [ ] Filter by FlowFile
- [ ] Export provenance data

**API Endpoints Used**:
- GET `/api/provenance/events` - List events
- GET `/api/provenance/events/{id}` - Event details
- GET `/api/provenance/lineage/{flowFileId}` - Lineage graph

**Expected Behavior**:
- Timeline displays chronological events
- Search filters events
- Lineage graph shows relationships

---

### 2.11 Error Handling
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] Backend unreachable
- [ ] Network timeout
- [ ] 404 Not Found
- [ ] 500 Server Error
- [ ] Invalid data format
- [ ] Validation errors
- [ ] Connection lost
- [ ] Retry mechanisms

**Expected Behavior**:
- User-friendly error messages
- No crashes or blank screens
- Ability to retry operations
- Fallback UI states

---

### 2.12 Performance
**Status**: ⏳ PENDING

#### Test Cases:
- [ ] Load time for flow list
- [ ] Load time for flow canvas
- [ ] Large flow (50+ processors)
- [ ] Many connections (100+)
- [ ] Canvas rendering performance
- [ ] Memory usage over time
- [ ] API response times

**Expected Behavior**:
- Pages load < 2 seconds
- Canvas remains responsive
- No memory leaks
- Smooth animations

---

## 3. Known Issues

### Issue 1: Empty Flow on Initial Load
**Status**: ✅ FIXED (2025-10-03)
**Problem**: Frontend expected `processors` and `connections` arrays but backend only returned `{id, name}`
**Solution**: Updated backend `HandleGetFlow` to include processors and connections

### Issue 2: Connections Not in ProcessGroup
**Status**: ✅ FIXED (2025-10-03)
**Problem**: Connections created but not added to ProcessGroup.Connections map
**Solution**: Updated `AddConnection` to add connection to root ProcessGroup

---

## 4. Test Results Summary

### Coverage by Feature Area
- [ ] Navigation: 0/10 (0%)
- [ ] Flows List: 0/6 (0%)
- [ ] Flow Canvas: 0/8 (0%)
- [ ] Component Palette: 0/5 (0%)
- [ ] Processor Operations: 0/7 (0%)
- [ ] Connection Operations: 0/6 (0%)
- [ ] Property Panel: 0/9 (0%)
- [ ] Toolbar: 0/8 (0%)
- [ ] Monitoring: 0/7 (0%)
- [ ] Provenance: 0/7 (0%)
- [ ] Error Handling: 0/8 (0%)
- [ ] Performance: 0/7 (0%)

### Overall Coverage: 0/88 (0%)

---

## 5. Testing Checklist

### Pre-Testing Setup
- [x] Backend running on port 8080
- [ ] Frontend running on port 3000
- [ ] Browser DevTools open
- [ ] Network tab recording
- [ ] Console tab visible

### During Testing
- [ ] Record all errors/warnings
- [ ] Screenshot broken functionality
- [ ] Note response times
- [ ] Document unexpected behavior
- [ ] Test in multiple browsers

### Post-Testing
- [ ] Document all findings
- [ ] Create bug tickets
- [ ] Update test results
- [ ] Calculate coverage percentage
- [ ] Recommend fixes/improvements

---

## 6. Next Steps

1. Start frontend development server
2. Begin systematic testing from section 2.1
3. Mark tests as pass/fail
4. Document issues in section 3
5. Update coverage percentages
6. Create prioritized fix list

# pprof Usage Guide

Quick reference for profiling the Excel processing application with Go's pprof.

## Quick Start

### 1. Enable HTTP pprof Server (Recommended)

```bash
export ENABLE_PPROF=true
export PPROF_PORT=6060
go run main.go
```

While the app is running, open: **http://localhost:6060/debug/pprof/**

### 2. Generate Profile Files

```bash
# CPU profile
export CPU_PROFILE=cpu.prof
go run main.go

# Memory profile
export MEM_PROFILE=mem.prof
go run main.go

# Both
export CPU_PROFILE=cpu.prof
export MEM_PROFILE=mem.prof
go run main.go
```

## Analyzing Profiles

### CPU Profile

```bash
# Interactive mode
go tool pprof cpu.prof

# Web interface
go tool pprof -http=:8080 cpu.prof

# Top 10 functions by CPU time
go tool pprof -top cpu.prof

# Generate call graph (requires graphviz)
go tool pprof -pdf cpu.prof > cpu.pdf
```

### Memory Profile

```bash
# Interactive mode
go tool pprof mem.prof

# Web interface
go tool pprof -http=:8080 mem.prof

# Top memory allocations
go tool pprof -top mem.prof

# Show allocations with call graph
go tool pprof -alloc_space -pdf mem.prof > mem_alloc.pdf

# Show in-use memory
go tool pprof -inuse_space -pdf mem.prof > mem_inuse.pdf
```

### Real-time Profiling (HTTP Server)

```bash
# CPU profile (30 seconds)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutines
go tool pprof http://localhost:6060/debug/pprof/goroutine

# All allocations
go tool pprof http://localhost:6060/debug/pprof/allocs
```

## pprof Interactive Commands

Once in interactive mode (`go tool pprof <profile>`):

```
top           - Show top 10 entries
top20         - Show top 20 entries
list <func>   - Show source code for function
web           - Open call graph in browser
pdf           - Generate PDF call graph
png           - Generate PNG call graph
quit          - Exit
help          - Show all commands
```

## Common Analysis Tasks

### Find Memory Leaks

```bash
# Take heap snapshot while app is running
go tool pprof http://localhost:6060/debug/pprof/heap

# In pprof interactive mode:
(pprof) top
(pprof) list <function_name>
```

### Find CPU Hotspots

```bash
# CPU profile for 30 seconds
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# In pprof interactive mode:
(pprof) top
(pprof) web
```

### Check Goroutine Leaks

```bash
go tool pprof http://localhost:6060/debug/pprof/goroutine

# In pprof interactive mode:
(pprof) top
```

## Example Workflow

### 1. Start Application with Profiling

```bash
# Terminal 1: Run application with pprof
export ENABLE_PPROF=true
export CPU_PROFILE=cpu.prof
export MEM_PROFILE=mem.prof
go run main.go
```

### 2. Monitor in Real-time

```bash
# Terminal 2: Watch heap allocations
watch -n 1 'curl -s http://localhost:6060/debug/pprof/heap | head -20'
```

### 3. Capture Profile During Processing

```bash
# Terminal 2: Capture 30-second CPU profile
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30
```

### 4. Analyze After Completion

```bash
# Analyze CPU profile
go tool pprof -http=:8080 cpu.prof

# Analyze memory profile
go tool pprof -http=:8080 mem.prof
```

## Web UI Features

When using `-http` flag, you get a web interface with:

- **Top**: Functions sorted by resource usage
- **Graph**: Call graph visualization
- **Flame Graph**: Hierarchical view of resource usage
- **Peek**: Source code view
- **Source**: Annotated source code

## Environment Variables Summary

```bash
# Enable HTTP pprof server
ENABLE_PPROF=true          # Default: false
PPROF_PORT=6060            # Default: 6060

# File-based profiling
CPU_PROFILE=cpu.prof       # Default: "" (disabled)
MEM_PROFILE=mem.prof       # Default: "" (disabled)
```

## Tips

1. **Use HTTP server for development**: Real-time profiling without restarting
2. **Use file profiles for production**: Capture profiles without HTTP server
3. **Profile during peak load**: Capture when processing large files
4. **Compare profiles**: Take multiple snapshots to track improvements
5. **Focus on top functions**: Usually top 5-10 functions matter most

## Troubleshooting

### Port already in use

```bash
# Change pprof port
export PPROF_PORT=6061
```

### Cannot open web UI

```bash
# Install graphviz (required for graphs)
# macOS
brew install graphviz

# Ubuntu/Debian
sudo apt-get install graphviz

# Then use -http flag
go tool pprof -http=:8080 cpu.prof
```

### Profile file not created

Check that environment variable is set:
```bash
echo $CPU_PROFILE
echo $MEM_PROFILE
```

## Advanced Usage

### Compare Two Profiles

```bash
# Take baseline
go tool pprof -proto http://localhost:6060/debug/pprof/heap > baseline.pb.gz

# After some time, take another snapshot
go tool pprof -proto http://localhost:6060/debug/pprof/heap > current.pb.gz

# Compare
go tool pprof -base=baseline.pb.gz current.pb.gz
```

### Filter by Function

```bash
# Show only specific package
go tool pprof -focus=main cpu.prof

# Exclude specific functions
go tool pprof -ignore=runtime cpu.prof
```

### Memory Allocation Types

```bash
# Allocated space (total allocations)
go tool pprof -alloc_space mem.prof

# Allocated objects (number of allocations)
go tool pprof -alloc_objects mem.prof

# In-use space (current memory usage)
go tool pprof -inuse_space mem.prof

# In-use objects (current object count)
go tool pprof -inuse_objects mem.prof
```

## Example Output Interpretation

### CPU Profile Top Output

```
Showing nodes accounting for 2.5s, 83.33% of 3s total
      flat  flat%   sum%        cum   cum%
     1.2s 40.00% 40.00%      1.5s 50.00%  main.(*ExampleProcessor).Process
     0.8s 26.67% 66.67%      0.9s 30.00%  runtime.mallocgc
     0.5s 16.67% 83.33%      0.5s 16.67%  github.com/xuri/excelize/v2.Rows.Next
```

- **flat**: Time spent in function itself
- **cum**: Cumulative time (function + callees)
- Focus on high **cum%** values

### Memory Profile Top Output

```
Showing nodes accounting for 512MB, 85.33% of 600MB total
      flat  flat%   sum%        cum   cum%
    256MB 42.67% 42.67%     300MB 50.00%  main.(*ExampleProcessor).Process
    128MB 21.33% 64.00%     150MB 25.00%  github.com/xuri/excelize/v2.OpenReader
    128MB 21.33% 85.33%     128MB 21.33%  bytes.Buffer.grow
```

- **flat**: Memory allocated by function itself
- **cum**: Cumulative allocations
- Focus on high **flat** values for memory leaks

## Resources

- [Official pprof Documentation](https://pkg.go.dev/net/http/pprof)
- [Go Blog: Profiling Go Programs](https://go.dev/blog/pprof)
- [pprof GitHub](https://github.com/google/pprof)

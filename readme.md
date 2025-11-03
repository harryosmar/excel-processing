
```shell
# Limit memory to 512MB
docker run --memory=512m -v $(pwd):/app golang:1.21 \
  sh -c "cd /app && go run main.go"
```

https://examplefile.com/document/xlsx/100-mb-xlsx#google_vignette
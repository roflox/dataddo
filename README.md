### Overview
Records are stored in the memory and every 500ms are saved to the file.  
Each operation (Save, Update, Delete, Get) is guarded with mutex lock.


### Params:
debug - prints state of database periodicaly  
db_file - path to file in which data are stored, if not specified ./records.bin is used


### How to run
#### Docker:

```
    docker build -t my-go-api .
    
    docker run -p 8080:8080 my-go-api
```

#### Without docker:
```
     go build -o myapi ./main
     
     ./myapi
```

### Tests

Use provided postman collection. It contains simple requests to the API.




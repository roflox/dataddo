package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type BinaryData interface {
	write(file *os.File) error
	read(recordBytes []byte) error
}

const recordSize = 8 + 8 + 64 + 1 + 16 + 1

type Record struct {
	ID        int64     `json:"ID"`
	IntValue  int64     `json:"IntValue"`
	StrValue  string    `json:"StrValue"`
	BoolValue bool      `json:"BoolValue"`
	TimeValue time.Time `json:"TimeValue"`
}

func (r *Record) write(file *os.File) error {

	buffer := make([]byte, 98)

	binary.LittleEndian.PutUint64(buffer, uint64(r.ID))
	binary.LittleEndian.PutUint64(buffer[8:], uint64(r.IntValue))

	copy(buffer[16:], r.StrValue)

	if r.BoolValue {
		buffer[80] = 1
	} else {
		buffer[80] = 0
	}

	timeBytes, err := r.TimeValue.MarshalBinary()
	if err != nil {
		return err
	}
	copy(buffer[81:], timeBytes)

	buffer[len(buffer)-1] = '\n'

	_, err = file.Write(buffer)
	return err
}

func (r *Record) read(recordBytes []byte) error {
	reader := bytes.NewReader(recordBytes)
	if err := binary.Read(reader, binary.LittleEndian, &r.ID); err != nil {
		return err
	}

	if err := binary.Read(reader, binary.LittleEndian, &r.IntValue); err != nil {
		return err
	}

	strBytes := make([]byte, 64)
	if _, err := reader.Read(strBytes); err != nil {
		return err
	}
	r.StrValue = string(bytes.TrimRight(strBytes, "\x00"))

	boolByte := make([]byte, 1)
	if _, err := reader.Read(boolByte); err != nil {
		return err
	}
	r.BoolValue = boolByte[0] != 0

	timeBytes := make([]byte, 16)
	_, err := reader.Read(timeBytes)
	if err != nil {
		return err
	}

	// Remove trailing 0x00 bytes
	timeBytes = bytes.TrimRight(timeBytes, string([]byte{0}))

	// Unmarshal the binary data into time.Time
	if err := r.TimeValue.UnmarshalBinary(timeBytes); err != nil {
		return err
	}
	return nil
}

type RecordRepository interface {
	add(record Record) (int64, error)
	read(id int64) (Record, error)
	update(record Record, id int64) error
	delete(id int64) error
	printDb()
	saveDb() error
	loadDb() error
}

type RecordRepositoryImpl struct {
	lock           sync.Mutex
	records        []Record
	deletedIndexes []int64
	realFilePath   *string
}

func (f *RecordRepositoryImpl) saveDb() error {
	f.lock.Lock()
	defer f.lock.Unlock()
	_, err := os.Stat(*f.realFilePath)

	//check if file exists, create new if not
	var file *os.File
	if err == nil {
		file, err = os.OpenFile(*f.realFilePath, os.O_WRONLY, os.ModeAppend)
	} else if os.IsNotExist(err) {
		file, err = os.Create(*f.realFilePath)
	}

	if err != nil {
		return err
	}
	defer file.Close()
	for i := range f.records {
		err := f.records[i].write(file)
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *RecordRepositoryImpl) loadDb() error {
	if f.realFilePath == nil {
		return nil
	}
	file, err := os.Open(*f.realFilePath)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	defer file.Close()

	buffer := make([]byte, recordSize)

	iterator := 0
	for {
		// Read a chunk of data into the buffer
		iterator++
		n, err := file.Read(buffer)
		if err == io.EOF {
			// EOF (end of file) reached
			break
		} else if err != nil {
			log.Fatal(err)
		}

		// Process the chunk of data (use the first 'n' bytes of the buffer)
		record := Record{}
		err = record.read((buffer[:n]))
		if record.ID == 0 {
			f.deletedIndexes = append(f.deletedIndexes, int64(iterator)-1)
		}
		if err != nil {
			return fmt.Errorf("error loading data")
		}
		f.records = append(f.records, record)
	}
	return nil
}

func (f *RecordRepositoryImpl) printDb() {
	f.lock.Lock()
	defer f.lock.Unlock()
	for i := range f.records {
		fmt.Println(f.records[i])
	}
}

func (f *RecordRepositoryImpl) add(record Record) (int64, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	var i int
	var replace bool
	if len(f.deletedIndexes) == 0 {
		i = len(f.records) + 1
		replace = false
	} else {
		i = int(f.deletedIndexes[0])
		f.deletedIndexes = f.deletedIndexes[1:]
		replace = true
	}

	record.ID = int64(i + 1)
	if replace {
		f.records[i] = record
	} else {
		f.records = append(f.records, record)
	}
	return record.ID, nil
}

func (f *RecordRepositoryImpl) read(id int64) (Record, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if !f.canExist(id) {
		return Record{}, fmt.Errorf("NOT_FOUND")
	}
	record := f.records[id]
	if record.ID == 0 {
		return Record{}, fmt.Errorf("NOT_FOUND")
	}
	return record, nil
}

func (f *RecordRepositoryImpl) canExist(id int64) bool {
	if id < 0 || id >= int64(len(f.records)) {
		return false
	}
	return true
}

func (f *RecordRepositoryImpl) update(record Record, id int64) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	record.ID = id
	if !f.canExist(id) {
		return fmt.Errorf("NOT_FOUND")
	}
	f.records[id-1] = record
	return nil
}

func (f *RecordRepositoryImpl) delete(id int64) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	if !f.canExist(id) || f.records[id].ID == 0 {
		return fmt.Errorf("NOT_FOUND")
	}
	f.records[id].ID = 0
	f.deletedIndexes = append(f.deletedIndexes, id)
	return nil
}

var FileDB RecordRepositoryImpl

func main() {

	debug := flag.Bool("debug", true, "sets log level to debug")
	dbFile := flag.String("db_file", "./records.bin", "path to db file")

	flag.Parse()

	FileDB = RecordRepositoryImpl{
		lock:           sync.Mutex{},
		records:        make([]Record, 0),
		realFilePath:   dbFile,
		deletedIndexes: make([]int64, 0),
	}

	abs, err := filepath.Abs(*FileDB.realFilePath)
	if err != nil {
		panic(err)
	}
	fmt.Println("dbFile", abs)
	FileDB.loadDb()
	if *debug {
		fmt.Println("---INIT STATE---")
		FileDB.printDb()
		fmt.Println("---INIT STATE---")
	}

	http.HandleFunc("/records", createRecord)
	http.HandleFunc("/records/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRecordByID(w, r)
		case http.MethodPut:
			updateRecordByID(w, r)
		case http.MethodDelete:
			deleteRecordByID(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		_, err := w.Write([]byte("OK"))
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		if !*debug {
			return
		}
		for {
			time.Sleep(10 * time.Second)
			fmt.Println("---START PRINTING DB---")
			FileDB.printDb()
			fmt.Println("---END PRINTING DB---")

		}
	}()

	go func() {
		for {
			time.Sleep(500 * time.Millisecond)
			err := FileDB.saveDb()
			if err != nil {
				log.Fatal(err)
				return
			}
		}
	}()

	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}

}

func getId(w http.ResponseWriter, r *http.Request) (int64, error) {
	idStr := r.URL.Path[len("/records/"):]
	parseInt, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid record ID", http.StatusBadRequest)
		return 0, err
	}
	return parseInt, nil
}

func deleteRecordByID(w http.ResponseWriter, r *http.Request) {
	id, err := getId(w, r)
	if err != nil {
		return
	}

	err = FileDB.delete(id - 1)
	if err != nil {
		handleStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)

}

func updateRecordByID(w http.ResponseWriter, r *http.Request) {
	id, err := getId(w, r)
	if err != nil {
		return
	}
	fmt.Println(id)

	var updateRecord Record
	if err := json.NewDecoder(r.Body).Decode(&updateRecord); err != nil {
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	err = FileDB.update(updateRecord, id-1)
	if err != nil {
		handleStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func getRecordByID(w http.ResponseWriter, r *http.Request) {
	id, err := getId(w, r)
	if err != nil {
		return
	}
	read, err := FileDB.read(id - 1)
	if err != nil {
		handleStorageError(w, err)
		return
	}

	marshal, err := json.Marshal(read)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	_, err = w.Write(marshal)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)

}

func createRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}

	var newRecord Record
	if err := json.NewDecoder(r.Body).Decode(&newRecord); err != nil {
		http.Error(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	_, err := FileDB.add(newRecord)
	if err != nil {
		return
	}

	w.WriteHeader(http.StatusCreated)

}

func handleStorageError(w http.ResponseWriter, err error) {
	switch err.Error() {
	case "NOT_FOUND":
		http.Error(w, "Not Found", http.StatusNotFound)
	default:
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

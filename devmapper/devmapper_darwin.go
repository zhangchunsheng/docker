package devmapper

import (
	"os"
)

type (
	Task struct {
	}
	Info struct {
		Exists        int
		Suspended     int
		LiveTable     int
		InactiveTable int
		OpenCount     int32
		EventNr       uint32
		Major         uint32
		Minor         uint32
		ReadOnly      int
		TargetCount   int32
	}
	TaskType        int
	DevmapperLogger interface {
		log(level int, file string, line int, dmError int, message string)
	}
)

func (t *Task) destroy() {
	panic("unimplemented on dawrin")
}

func TaskCreate(tasktype TaskType) *Task {
	panic("unimplemented on dawrin")
}

func (t *Task) Run() error {
	panic("unimplemented on dawrin")
}

func (t *Task) SetName(name string) error {
	panic("unimplemented on dawrin")
}

func (t *Task) SetMessage(message string) error {
	panic("unimplemented on dawrin")
}

func (t *Task) SetSector(sector uint64) error {
	panic("unimplemented on dawrin")
}

func (t *Task) SetCookie(cookie *uint32, flags uint16) error {
	panic("unimplemented on dawrin")
}

func (t *Task) SetRo() error {
	panic("unimplemented on dawrin")
}

func (t *Task) AddTarget(start uint64, size uint64, ttype string, params string) error {
	panic("unimplemented on dawrin")
}

func (t *Task) GetDriverVersion() (string, error) {
	panic("unimplemented on dawrin")
}

func (t *Task) GetInfo() (*Info, error) {
	panic("unimplemented on dawrin")
}

func (t *Task) GetNextTarget(next uintptr) (uintptr, uint64, uint64, string, string) {
	panic("unimplemented on dawrin")
}

func AttachLoopDevice(filename string) (*os.File, error) {
	panic("unimplemented on dawrin")
}

func getBlockSize(fd uintptr) int {
	panic("unimplemented on dawrin")
}

func GetBlockDeviceSize(file *os.File) (uint64, error) {
	panic("unimplemented on dawrin")
}

func UdevWait(cookie uint32) error {
	panic("unimplemented on dawrin")
}

func LogInitVerbose(level int) {
	panic("unimplemented on dawrin")
}

func logInit(logger DevmapperLogger) {
	panic("unimplemented on dawrin")
}

func SetDevDir(dir string) error {
	panic("unimplemented on dawrin")
}

func GetLibraryVersion() (string, error) {
	panic("unimplemented on dawrin")
}

// Useful helper for cleanup
func RemoveDevice(name string) error {
	panic("unimplemented on dawrin")
}

func createPool(poolName string, dataFile *os.File, metadataFile *os.File) error {
	panic("unimplemented on dawrin")
}

func createTask(t TaskType, name string) (*Task, error) {
	panic("unimplemented on dawrin")
}

func getInfo(name string) (*Info, error) {
	panic("unimplemented on dawrin")
}

func getStatus(name string) (uint64, uint64, string, string, error) {
	panic("unimplemented on dawrin")
}

func setTransactionId(poolName string, oldId uint64, newId uint64) error {
	panic("unimplemented on dawrin")
}

func suspendDevice(name string) error {
	panic("unimplemented on dawrin")
}

func resumeDevice(name string) error {
	panic("unimplemented on dawrin")
}

func createDevice(poolName string, deviceId int) error {
	panic("unimplemented on dawrin")
}

func deleteDevice(poolName string, deviceId int) error {
	panic("unimplemented on dawrin")
}

func removeDevice(name string) error {
	panic("unimplemented on dawrin")
}

func activateDevice(poolName string, name string, deviceId int, size uint64) error {
	panic("unimplemented on dawrin")
}

func (devices *DeviceSetDM) createSnapDevice(poolName string, deviceId int, baseName string, baseDeviceId int) error {
	panic("unimplemented on dawrin")
}

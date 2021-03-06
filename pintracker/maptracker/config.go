package maptracker

import (
	"encoding/json"
	"errors"

	"github.com/ipfs/ipfs-cluster/config"
)

const configKey = "maptracker"

// Default values for this Config.
const (
	DefaultMaxPinQueueSize = 50000
	DefaultConcurrentPins  = 10
)

// Config allows to initialize a Monitor and customize some parameters.
type Config struct {
	config.Saver

	// If higher, they will automatically marked with an error.
	MaxPinQueueSize int
	// ConcurrentPins specifies how many pin requests can be sent to the ipfs
	// daemon in parallel. If the pinning method is "refs", it might increase
	// speed. Unpin requests are always processed one by one.
	ConcurrentPins int
}

type jsonConfig struct {
	MaxPinQueueSize int `json:"max_pin_queue_size"`
	ConcurrentPins  int `json:"concurrent_pins"`
}

// ConfigKey provides a human-friendly identifier for this type of Config.
func (cfg *Config) ConfigKey() string {
	return configKey
}

// Default sets the fields of this Config to sensible values.
func (cfg *Config) Default() error {
	cfg.MaxPinQueueSize = DefaultMaxPinQueueSize
	cfg.ConcurrentPins = DefaultConcurrentPins
	return nil
}

// Validate checks that the fields of this Config have working values,
// at least in appearance.
func (cfg *Config) Validate() error {
	if cfg.MaxPinQueueSize <= 0 {
		return errors.New("maptracker.max_pin_queue_size too low")
	}

	if cfg.ConcurrentPins <= 0 {
		return errors.New("maptracker.concurrent_pins is too low")
	}
	return nil
}

// LoadJSON sets the fields of this Config to the values defined by the JSON
// representation of it, as generated by ToJSON.
func (cfg *Config) LoadJSON(raw []byte) error {
	jcfg := &jsonConfig{}
	err := json.Unmarshal(raw, jcfg)
	if err != nil {
		logger.Error("Error unmarshaling maptracker config")
		return err
	}

	cfg.Default()

	config.SetIfNotDefault(jcfg.MaxPinQueueSize, &cfg.MaxPinQueueSize)
	config.SetIfNotDefault(jcfg.ConcurrentPins, &cfg.ConcurrentPins)

	return cfg.Validate()
}

// ToJSON generates a human-friendly JSON representation of this Config.
func (cfg *Config) ToJSON() ([]byte, error) {
	jcfg := &jsonConfig{}

	jcfg.MaxPinQueueSize = cfg.MaxPinQueueSize
	jcfg.ConcurrentPins = cfg.ConcurrentPins

	return config.DefaultJSONMarshal(jcfg)
}

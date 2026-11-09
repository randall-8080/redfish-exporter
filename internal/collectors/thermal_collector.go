package collectors

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

type ThermalCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors
	tempDesc                  *prometheus.Desc
	tempStatusDesc            *prometheus.Desc
	tempThresholdDesc         *prometheus.Desc
	fanDesc                   *prometheus.Desc
	fanStatusDesc             *prometheus.Desc
	fanThresholdDesc          *prometheus.Desc
	fanSpeedPercentDesc       *prometheus.Desc
	humidityDesc              *prometheus.Desc
	humidityThresholdDesc     *prometheus.Desc
	thermalSubsystemHealthDec *prometheus.Desc
}

func NewThermalCollector(target string, client *gofish.APIClient, logger *slog.Logger) *ThermalCollector {
	thermalBaseLabels := []string{"chassis_id"}

	return &ThermalCollector{
		target: target,
		client: client,
		logger: logger,

		tempDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "temperature", "celsius"),
			"Redfish thermal temperature readings in Celsius",
			append(thermalBaseLabels, "sensor", "physical_context"), prometheus.Labels{"target": target},
		),
		tempStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "temperature", "health"),
			"Redfish thermal temperature health (1=OK, 0=not OK)",
			append(thermalBaseLabels, "sensor", "physical_context", "health"), prometheus.Labels{"target": target},
		),
		tempThresholdDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "temperature", "threshold_celsius"),
			"Redfish temperature sensor threshold in Celsius",
			append(thermalBaseLabels, "sensor", "physical_context", "level"), prometheus.Labels{"target": target},
		),
		fanDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "fan", "rpm"),
			"Redfish thermal fan speed in RPM",
			append(thermalBaseLabels, "fan", "physical_context", "unit"), prometheus.Labels{"target": target},
		),
		fanStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "fan", "health"),
			"Redfish thermal fan health (1=OK, 0=not OK)",
			append(thermalBaseLabels, "fan", "physical_context", "health"), prometheus.Labels{"target": target},
		),
		fanThresholdDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "fan", "threshold_rpm"),
			"Redfish fan sensor threshold in RPM",
			append(thermalBaseLabels, "fan", "physical_context", "level"), prometheus.Labels{"target": target},
		),
		fanSpeedPercentDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "fan", "speed_percent"),
			"Redfish fan speed in percent (0-100)",
			append(thermalBaseLabels, "fan", "physical_context"), prometheus.Labels{"target": target},
		),
		humidityDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "humidity", "percent"),
			"Redfish relative humidity in percent",
			append(thermalBaseLabels, "sensor", "physical_context"), prometheus.Labels{"target": target},
		),
		humidityThresholdDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "humidity", "threshold_percent"),
			"Redfish humidity sensor threshold in percent",
			append(thermalBaseLabels, "sensor", "physical_context", "level"), prometheus.Labels{"target": target},
		),
		thermalSubsystemHealthDec: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "thermal_subsystem", "health"),
			"Health of the thermal subsystem (1=OK, 0=not OK)",
			append(thermalBaseLabels, "health"), prometheus.Labels{"target": target},
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected by this Collector.
func (c *ThermalCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.tempDesc
	ch <- c.tempStatusDesc
	ch <- c.tempThresholdDesc
	ch <- c.fanDesc
	ch <- c.fanStatusDesc
	ch <- c.fanThresholdDesc
	ch <- c.fanSpeedPercentDesc
	ch <- c.humidityDesc
	ch <- c.humidityThresholdDesc
	ch <- c.thermalSubsystemHealthDec
}

// Collect satisfies prometheus.Collector; use redfish_collector_success to detect failures.
// When run standalone it creates its own scrape context.
func (c *ThermalCollector) Collect(ch chan<- prometheus.Metric) {
	_ = c.collect(newScrapeContext(c.client, c.logger, false), ch)
}

// collect walks all chassis and reports an error when any resource fetch
// failed; metrics collected from the remaining resources are still emitted.
func (c *ThermalCollector) collect(ctx *scrapeContext, ch chan<- prometheus.Metric) error {
	chassisList, err := ctx.Chassis()
	if err != nil {
		c.logger.Error("failed to get chassis", "error", err, "fetched", len(chassisList), "target", c.target)
		if len(chassisList) == 0 {
			return fmt.Errorf("get chassis: %w", err)
		}
	}
	errs := []error{err}
	for _, chassis := range chassisList {
		if cerr := c.collectChassis(ctx, chassis, ch); cerr != nil {
			errs = append(errs, cerr)
		}
	}
	return errors.Join(errs...)
}

// collectChassis gathers every thermal source a chassis advertises and emits
// metrics per family using source ladders: modern sources first, legacy
// resources as fallback. Only the first source that yields a reading for a
// family is used, which prevents duplicate samples when the same physical
// sensor is represented in several resources.
func (c *ThermalCollector) collectChassis(ctx *scrapeContext, chassis *schemas.Chassis, ch chan<- prometheus.Metric) error {
	var errs []error

	legacyThermal, err := chassis.Thermal()
	switch {
	case err == nil:
	case isNotFound(err):
		// The service links Thermal but does not implement it; treat the
		// deprecated resource as absent and rely on modern sources.
		c.logger.Debug("thermal resource not found", "chassis", chassis.ID, "target", c.target)
	default:
		c.logger.Error("failed to get thermal resource", "error", err, "chassis", chassis.ID, "target", c.target)
		errs = append(errs, err)
	}

	sensors, err := ctx.Sensors(chassis)
	if err != nil {
		// gofish returns the sensors it could fetch; keep collecting them.
		c.logger.Error("failed to get sensors", "error", err, "chassis", chassis.ID, "target", c.target)
		errs = append(errs, err)
	}
	sensorsByURI := make(map[string]*schemas.Sensor, len(sensors))
	var tempSensors, rotationalSensors, humiditySensors []*schemas.Sensor
	for _, s := range sensors {
		sensorsByURI[s.ODataID] = s
		switch s.ReadingType {
		case schemas.TemperatureReadingType:
			tempSensors = append(tempSensors, s)
		case schemas.RotationalReadingType:
			rotationalSensors = append(rotationalSensors, s)
		case schemas.HumidityReadingType:
			humiditySensors = append(humiditySensors, s)
		}
	}

	var fans []*schemas.Fan
	var thermalMetrics *schemas.ThermalMetrics
	subsystem, err := chassis.ThermalSubsystem()
	switch {
	case err == nil && subsystem != nil:
		if subsystem.Status.Health != "" {
			ch <- prometheus.MustNewConstMetric(
				c.thermalSubsystemHealthDec,
				prometheus.GaugeValue,
				healthToFloat(subsystem.Status.Health),
				chassis.ID, string(subsystem.Status.Health),
			)
		}
		fans, err = subsystem.Fans()
		if err != nil {
			c.logger.Error("failed to get thermal subsystem fans", "error", err, "chassis", chassis.ID, "target", c.target)
			errs = append(errs, err)
		}
		thermalMetrics, err = subsystem.ThermalMetrics()
		switch {
		case err == nil:
		case isNotFound(err):
			c.logger.Debug("thermal metrics not found", "chassis", chassis.ID, "target", c.target)
		default:
			c.logger.Error("failed to get thermal metrics", "error", err, "chassis", chassis.ID, "target", c.target)
			errs = append(errs, err)
		}
	case err == nil:
		// No ThermalSubsystem link; legacy and sensor sources still apply.
	case isNotFound(err):
		c.logger.Debug("thermal subsystem not found", "chassis", chassis.ID, "target", c.target)
	default:
		c.logger.Error("failed to get thermal subsystem", "error", err, "chassis", chassis.ID, "target", c.target)
		errs = append(errs, err)
	}

	envMetrics, err := ctx.EnvironmentMetrics(chassis)
	switch {
	case err == nil:
	case isNotFound(err):
		// EnvironmentMetrics is license-restricted on some vendors; treat it
		// as a capability gap.
		c.logger.Debug("environment metrics not found", "chassis", chassis.ID, "target", c.target)
	default:
		c.logger.Error("failed to get environment metrics", "error", err, "chassis", chassis.ID, "target", c.target)
		errs = append(errs, err)
	}

	// Temperature readings: sensors → thermal metrics excerpts → environment
	// metrics excerpt → legacy Thermal. Readings reported by the power
	// collector as power supply temperatures are skipped.
	var psuConsumed map[string]bool
	if ctx.powerEnabled {
		psuConsumed = ctx.PSUConsumedTemperatureURIs(chassis)
	}
	temps := temperatureSamplesFromSensors(tempSensors, psuConsumed)
	if len(temps) == 0 {
		temps = temperatureSamplesFromThermalMetrics(thermalMetrics, sensorsByURI, psuConsumed)
	}
	if len(temps) == 0 {
		temps = temperatureSamplesFromEnvironment(envMetrics, sensorsByURI, psuConsumed)
	}
	if len(temps) == 0 {
		temps = temperatureSamplesFromLegacy(legacyThermal)
	}
	c.emitTemperatures(chassis.ID, temps, ch)

	// Fan speed: sensors → fan excerpts → legacy Thermal (RPM-unit fans).
	fanRPMs := fanReadingsFromSensors(rotationalSensors)
	if len(fanRPMs) == 0 {
		fanRPMs = fanReadingsFromFans(fans)
	}
	if len(fanRPMs) == 0 {
		fanRPMs = fanReadingsFromLegacyRPM(legacyThermal)
	}
	c.emitFanReadings(chassis.ID, fanRPMs, ch)

	// Fan health: fan resources → sensors → legacy Thermal.
	fanHealths := fanHealthsFromFans(fans)
	if len(fanHealths) == 0 {
		fanHealths = fanHealthsFromSensors(rotationalSensors)
	}
	if len(fanHealths) == 0 {
		fanHealths = fanHealthsFromLegacy(legacyThermal)
	}
	c.emitFanHealths(chassis.ID, fanHealths, ch)

	// Fan speed percent: fan resources → environment metrics → legacy
	// Thermal (percent-unit fans).
	fanPercents := fanPercentsFromFans(fans)
	if len(fanPercents) == 0 {
		fanPercents = fanPercentsFromEnvironment(envMetrics)
	}
	if len(fanPercents) == 0 {
		fanPercents = fanPercentsFromLegacy(legacyThermal)
	}
	c.emitFanPercents(chassis.ID, fanPercents, ch)

	// Humidity: sensors → environment metrics excerpt.
	humidity := humiditySamplesFromSensors(humiditySensors)
	if len(humidity) == 0 {
		humidity = humiditySamplesFromEnvironment(envMetrics, sensorsByURI)
	}
	c.emitHumidity(chassis.ID, humidity, ch)

	return errors.Join(errs...)
}

// temperatureSample is a temperature reading candidate from any source.
type temperatureSample struct {
	sensor     string
	sensorID   string // stable ID used to disambiguate label collisions
	context    string
	reading    float64
	health     schemas.Health
	emitHealth bool
	thresholds []thresholdValue
}

// fanReading is a fan RPM reading candidate from any source.
type fanReading struct {
	fan        string
	fanID      string
	context    string
	rpm        float64
	unit       string
	thresholds []thresholdValue
}

// fanHealth is a fan health candidate from any source.
type fanHealth struct {
	fan     string
	fanID   string
	context string
	health  schemas.Health
}

// fanPercent is a fan speed percent reading candidate from any source.
type fanPercent struct {
	fan     string
	fanID   string
	context string
	percent float64
}

// humiditySample is a relative humidity reading candidate from any source.
type humiditySample struct {
	sensor     string
	sensorID   string
	context    string
	percent    float64
	thresholds []thresholdValue
}

// thresholdValue is a single sensor threshold.
type thresholdValue struct {
	level string
	value float64
}

// thresholdLevels maps sensor Thresholds fields to metric label values in a
// fixed order so output is deterministic. User-adjustable variants are not
// exported.
var thresholdLevels = []struct {
	label string
	get   func(schemas.Thresholds) *float64
}{
	{"lower_fatal", func(t schemas.Thresholds) *float64 { return t.LowerFatal.Reading }},
	{"lower_critical", func(t schemas.Thresholds) *float64 { return t.LowerCritical.Reading }},
	{"lower_caution", func(t schemas.Thresholds) *float64 { return t.LowerCaution.Reading }},
	{"upper_caution", func(t schemas.Thresholds) *float64 { return t.UpperCaution.Reading }},
	{"upper_critical", func(t schemas.Thresholds) *float64 { return t.UpperCritical.Reading }},
	{"upper_fatal", func(t schemas.Thresholds) *float64 { return t.UpperFatal.Reading }},
}

func thresholdValues(t schemas.Thresholds) []thresholdValue {
	var out []thresholdValue
	for _, l := range thresholdLevels {
		if r := l.get(t); r != nil {
			out = append(out, thresholdValue{level: l.label, value: *r})
		}
	}
	return out
}

// sensorName returns the display name of a sensor resource, falling back to
// its stable ID when the service did not provide a name.
func sensorName(s *schemas.Sensor) string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID
}

// fanName returns the display name of a fan resource, falling back to its
// stable ID when the service did not provide a name.
func fanName(f *schemas.Fan) string {
	if f.Name != "" {
		return f.Name
	}
	return f.ID
}

// uniqueLabel returns the label to use for name. When a different source
// already claimed the same name, the stable ID is appended so distinct
// sensors with identical names are preserved instead of merged. ok is false
// when no unique label can be produced.
func uniqueLabel(seen map[string]string, name, id string) (string, bool) {
	if owner, ok := seen[name]; ok {
		name = fmt.Sprintf("%s (%s)", name, id)
		if _, ok := seen[name]; ok || owner == id {
			return "", false
		}
	}
	seen[name] = id
	return name, true
}

// temperatureSamplesFromSensors builds samples from Temperature-type sensor
// resources. Disabled and absent sensors and sensors without a reading are
// skipped; real zero and negative readings are preserved.
func temperatureSamplesFromSensors(sensors []*schemas.Sensor, skip map[string]bool) []temperatureSample {
	var out []temperatureSample
	for _, s := range sensors {
		if s.Reading == nil || s.Status.State == schemas.DisabledState || s.Status.State == schemas.AbsentState {
			continue
		}
		if skip[s.ODataID] {
			continue
		}
		out = append(out, temperatureSample{
			sensor:     sensorName(s),
			sensorID:   s.ID,
			context:    string(s.PhysicalContext),
			reading:    *s.Reading,
			health:     s.Status.Health,
			emitHealth: s.Status.State == schemas.EnabledState,
			thresholds: thresholdValues(s.Thresholds),
		})
	}
	return out
}

// temperatureSamplesFromThermalMetrics builds samples from the excerpt lists
// of a ThermalMetrics resource. Excerpts whose DataSourceUri matches an
// excerpt already emitted are skipped; summary readings without a link are
// skipped when the readings list already produced samples, since they
// summarize the same sensors.
func temperatureSamplesFromThermalMetrics(m *schemas.ThermalMetrics, sensorsByURI map[string]*schemas.Sensor, skip map[string]bool) []temperatureSample {
	if m == nil {
		return nil
	}
	var out []temperatureSample
	emitted := make(map[string]bool)
	for _, tr := range m.TemperatureReadingsCelsius {
		if tr.Reading == nil {
			continue
		}
		if tr.DataSourceURI != "" && (emitted[tr.DataSourceURI] || skip[tr.DataSourceURI]) {
			continue
		}
		sensor, id := excerptName(tr.DeviceName, tr.DataSourceURI, sensorsByURI)
		if sensor == "" {
			continue
		}
		if tr.DataSourceURI != "" {
			emitted[tr.DataSourceURI] = true
		}
		out = append(out, temperatureSample{
			sensor:   sensor,
			sensorID: id,
			context:  string(tr.PhysicalContext),
			reading:  *tr.Reading,
		})
	}
	readingsYielded := len(out) > 0
	for _, role := range []struct {
		name    string
		excerpt schemas.SensorExcerpt
	}{
		{"Intake", m.TemperatureSummaryCelsius.Intake},
		{"Exhaust", m.TemperatureSummaryCelsius.Exhaust},
		{"Ambient", m.TemperatureSummaryCelsius.Ambient},
		{"Internal", m.TemperatureSummaryCelsius.Internal},
	} {
		if role.excerpt.Reading == nil {
			continue
		}
		if role.excerpt.DataSourceURI != "" && (emitted[role.excerpt.DataSourceURI] || skip[role.excerpt.DataSourceURI]) {
			continue
		}
		if role.excerpt.DataSourceURI == "" && readingsYielded {
			// Summary entries without a link summarize the readings list.
			continue
		}
		sensor, id := excerptName(role.name, role.excerpt.DataSourceURI, sensorsByURI)
		if sensor == "" {
			continue
		}
		if role.excerpt.DataSourceURI != "" {
			emitted[role.excerpt.DataSourceURI] = true
		}
		out = append(out, temperatureSample{sensor: sensor, sensorID: id, reading: *role.excerpt.Reading})
	}
	return out
}

// temperatureSamplesFromEnvironment builds a sample from the chassis
// EnvironmentMetrics temperature excerpt. TemperatureCelsius is preferred;
// newer services that deprecated it report AmbientTemperatureCelsius.
func temperatureSamplesFromEnvironment(e *schemas.EnvironmentMetrics, sensorsByURI map[string]*schemas.Sensor, skip map[string]bool) []temperatureSample {
	if e == nil {
		return nil
	}
	excerpt := e.TemperatureCelsius
	if excerpt.Reading == nil {
		excerpt = e.AmbientTemperatureCelsius
	}
	if excerpt.Reading == nil || skip[excerpt.DataSourceURI] {
		return nil
	}
	sensor, id := excerptName("EnvironmentMetrics", excerpt.DataSourceURI, sensorsByURI)
	return []temperatureSample{{sensor: sensor, sensorID: id, reading: *excerpt.Reading}}
}

// temperatureSamplesFromLegacy builds samples from the deprecated Thermal
// resource, preserving the previous export semantics.
func temperatureSamplesFromLegacy(t *schemas.Thermal) []temperatureSample {
	if t == nil {
		return nil
	}
	var out []temperatureSample
	for _, temp := range t.Temperatures {
		if temp.Status.State == schemas.DisabledState || temp.Status.State == schemas.AbsentState {
			continue
		}
		if temp.ReadingCelsius == nil {
			continue
		}
		out = append(out, temperatureSample{
			sensor:     temp.Name,
			sensorID:   temp.MemberID,
			context:    string(temp.PhysicalContext),
			reading:    *temp.ReadingCelsius,
			health:     temp.Status.Health,
			emitHealth: temp.Status.State == schemas.EnabledState,
		})
	}
	return out
}

// excerptName resolves the sensor label for an excerpt: the embedded device
// name if present, otherwise the name of the sensor its DataSourceUri
// references, otherwise the last segment of the URI.
func excerptName(deviceName, uri string, sensorsByURI map[string]*schemas.Sensor) (string, string) {
	if uri != "" {
		if s, ok := sensorsByURI[uri]; ok {
			return sensorName(s), s.ID
		}
	}
	if deviceName != "" {
		return deviceName, uri
	}
	if uri != "" {
		for i := len(uri) - 1; i >= 0; i-- {
			if uri[i] == '/' {
				return uri[i+1:], uri
			}
		}
	}
	return "", ""
}

// fanReadingsFromSensors builds RPM readings from Rotational-type sensor
// resources. Reading is preferred; SpeedRPM is used as a fallback.
func fanReadingsFromSensors(sensors []*schemas.Sensor) []fanReading {
	var out []fanReading
	for _, s := range sensors {
		if s.Status.State == schemas.DisabledState || s.Status.State == schemas.AbsentState {
			continue
		}
		reading := s.Reading
		if reading == nil {
			reading = s.SpeedRPM
		}
		if reading == nil {
			continue
		}
		out = append(out, fanReading{
			fan:        sensorName(s),
			fanID:      s.ID,
			context:    string(s.PhysicalContext),
			rpm:        *reading,
			unit:       string(schemas.RPMReadingUnits),
			thresholds: thresholdValues(s.Thresholds),
		})
	}
	return out
}

// fanReadingsFromFans builds RPM readings from the SpeedRPM field of fan
// resource excerpts.
func fanReadingsFromFans(fans []*schemas.Fan) []fanReading {
	var out []fanReading
	for _, f := range fans {
		if f.Status.State == schemas.AbsentState {
			continue
		}
		if f.SpeedPercent.SpeedRPM == nil {
			continue
		}
		out = append(out, fanReading{
			fan:     fanName(f),
			fanID:   f.ID,
			context: string(f.PhysicalContext),
			rpm:     *f.SpeedPercent.SpeedRPM,
			unit:    string(schemas.RPMReadingUnits),
		})
	}
	return out
}

// fanReadingsFromLegacyRPM builds RPM readings from legacy Thermal fans that
// report RPM units. Percent-unit fans are exported as fan speed percent
// instead, never as RPM.
func fanReadingsFromLegacyRPM(t *schemas.Thermal) []fanReading {
	if t == nil {
		return nil
	}
	var out []fanReading
	for _, f := range t.Fans {
		if f.Status.State == schemas.AbsentState || f.Name == "" || f.Reading == nil {
			continue
		}
		if f.ReadingUnits == schemas.PercentReadingUnits {
			continue
		}
		out = append(out, fanReading{
			fan:     f.Name,
			fanID:   f.MemberID,
			context: string(f.PhysicalContext),
			rpm:     float64(*f.Reading),
			unit:    string(f.ReadingUnits),
		})
	}
	return out
}

// fanHealthsFromFans builds health samples from thermal subsystem fan
// resources, the canonical fan health source.
func fanHealthsFromFans(fans []*schemas.Fan) []fanHealth {
	var out []fanHealth
	for _, f := range fans {
		if f.Status.State != schemas.EnabledState {
			continue
		}
		out = append(out, fanHealth{
			fan:     fanName(f),
			fanID:   f.ID,
			context: string(f.PhysicalContext),
			health:  f.Status.Health,
		})
	}
	return out
}

// fanHealthsFromSensors builds health samples from Rotational-type sensor
// resources. Health is independent of reading availability.
func fanHealthsFromSensors(sensors []*schemas.Sensor) []fanHealth {
	var out []fanHealth
	for _, s := range sensors {
		if s.Status.State != schemas.EnabledState {
			continue
		}
		out = append(out, fanHealth{
			fan:     sensorName(s),
			fanID:   s.ID,
			context: string(s.PhysicalContext),
			health:  s.Status.Health,
		})
	}
	return out
}

// fanHealthsFromLegacy builds health samples from legacy Thermal fans,
// preserving the previous export semantics.
func fanHealthsFromLegacy(t *schemas.Thermal) []fanHealth {
	if t == nil {
		return nil
	}
	var out []fanHealth
	for _, f := range t.Fans {
		if f.Status.State == schemas.AbsentState || f.Name == "" {
			continue
		}
		if f.Status.State != schemas.EnabledState {
			continue
		}
		out = append(out, fanHealth{
			fan:     f.Name,
			fanID:   f.MemberID,
			context: string(f.PhysicalContext),
			health:  f.Status.Health,
		})
	}
	return out
}

// fanPercentsFromFans builds percent readings from fan resource excerpts.
// Secondary rotor speeds are not exported; there is no label to tell the
// rotors apart.
func fanPercentsFromFans(fans []*schemas.Fan) []fanPercent {
	var out []fanPercent
	for _, f := range fans {
		if f.Status.State == schemas.AbsentState {
			continue
		}
		if f.SpeedPercent.Reading == nil {
			continue
		}
		out = append(out, fanPercent{
			fan:     fanName(f),
			fanID:   f.ID,
			context: string(f.PhysicalContext),
			percent: *f.SpeedPercent.Reading,
		})
	}
	return out
}

// fanPercentsFromEnvironment builds percent readings from the EnvironmentMetrics
// fan speed list.
func fanPercentsFromEnvironment(e *schemas.EnvironmentMetrics) []fanPercent {
	if e == nil {
		return nil
	}
	var out []fanPercent
	for _, fs := range e.FanSpeedsPercent {
		if fs.Reading == nil || fs.DeviceName == "" {
			continue
		}
		out = append(out, fanPercent{
			fan:     fs.DeviceName,
			fanID:   fs.DeviceName,
			context: string(fs.PhysicalContext),
			percent: *fs.Reading,
		})
	}
	return out
}

// fanPercentsFromLegacy builds percent readings from legacy Thermal fans
// that report Percent units. These were previously mislabeled as RPM.
func fanPercentsFromLegacy(t *schemas.Thermal) []fanPercent {
	if t == nil {
		return nil
	}
	var out []fanPercent
	for _, f := range t.Fans {
		if f.Status.State == schemas.AbsentState || f.Name == "" || f.Reading == nil {
			continue
		}
		if f.ReadingUnits != schemas.PercentReadingUnits {
			continue
		}
		out = append(out, fanPercent{
			fan:     f.Name,
			fanID:   f.MemberID,
			context: string(f.PhysicalContext),
			percent: float64(*f.Reading),
		})
	}
	return out
}

// humiditySamplesFromSensors builds samples from Humidity-type sensor
// resources.
func humiditySamplesFromSensors(sensors []*schemas.Sensor) []humiditySample {
	var out []humiditySample
	for _, s := range sensors {
		if s.Reading == nil || s.Status.State == schemas.DisabledState || s.Status.State == schemas.AbsentState {
			continue
		}
		out = append(out, humiditySample{
			sensor:     sensorName(s),
			sensorID:   s.ID,
			context:    string(s.PhysicalContext),
			percent:    *s.Reading,
			thresholds: thresholdValues(s.Thresholds),
		})
	}
	return out
}

// humiditySamplesFromEnvironment builds a sample from the EnvironmentMetrics
// humidity excerpt.
func humiditySamplesFromEnvironment(e *schemas.EnvironmentMetrics, sensorsByURI map[string]*schemas.Sensor) []humiditySample {
	if e == nil || e.HumidityPercent.Reading == nil {
		return nil
	}
	sensor, id := excerptName("EnvironmentMetrics", e.HumidityPercent.DataSourceURI, sensorsByURI)
	return []humiditySample{{sensor: sensor, sensorID: id, percent: *e.HumidityPercent.Reading}}
}

func (c *ThermalCollector) emitTemperatures(chassisID string, samples []temperatureSample, ch chan<- prometheus.Metric) {
	seen := make(map[string]string)
	for _, s := range samples {
		label, ok := uniqueLabel(seen, s.sensor, s.sensorID)
		if !ok {
			c.logger.Warn("skipping temperature reading with duplicate labels", "sensor", s.sensor, "sensor_id", s.sensorID, "chassis", chassisID, "target", c.target)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.tempDesc, prometheus.GaugeValue, s.reading, chassisID, label, s.context)
		if s.emitHealth {
			ch <- prometheus.MustNewConstMetric(c.tempStatusDesc, prometheus.GaugeValue, healthToFloat(s.health), chassisID, label, s.context, string(s.health))
		}
		for _, th := range s.thresholds {
			ch <- prometheus.MustNewConstMetric(c.tempThresholdDesc, prometheus.GaugeValue, th.value, chassisID, label, s.context, th.level)
		}
	}
}

func (c *ThermalCollector) emitFanReadings(chassisID string, readings []fanReading, ch chan<- prometheus.Metric) {
	seen := make(map[string]string)
	for _, r := range readings {
		label, ok := uniqueLabel(seen, r.fan, r.fanID)
		if !ok {
			c.logger.Warn("skipping fan reading with duplicate labels", "fan", r.fan, "fan_id", r.fanID, "chassis", chassisID, "target", c.target)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.fanDesc, prometheus.GaugeValue, r.rpm, chassisID, label, r.context, r.unit)
		for _, th := range r.thresholds {
			ch <- prometheus.MustNewConstMetric(c.fanThresholdDesc, prometheus.GaugeValue, th.value, chassisID, label, r.context, th.level)
		}
	}
}

func (c *ThermalCollector) emitFanHealths(chassisID string, healths []fanHealth, ch chan<- prometheus.Metric) {
	seen := make(map[string]string)
	for _, h := range healths {
		label, ok := uniqueLabel(seen, h.fan, h.fanID)
		if !ok {
			c.logger.Warn("skipping fan health with duplicate labels", "fan", h.fan, "fan_id", h.fanID, "chassis", chassisID, "target", c.target)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.fanStatusDesc, prometheus.GaugeValue, healthToFloat(h.health), chassisID, label, h.context, string(h.health))
	}
}

func (c *ThermalCollector) emitFanPercents(chassisID string, percents []fanPercent, ch chan<- prometheus.Metric) {
	seen := make(map[string]string)
	for _, p := range percents {
		label, ok := uniqueLabel(seen, p.fan, p.fanID)
		if !ok {
			c.logger.Warn("skipping fan speed percent with duplicate labels", "fan", p.fan, "chassis", chassisID, "target", c.target)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.fanSpeedPercentDesc, prometheus.GaugeValue, p.percent, chassisID, label, p.context)
	}
}

func (c *ThermalCollector) emitHumidity(chassisID string, samples []humiditySample, ch chan<- prometheus.Metric) {
	seen := make(map[string]string)
	for _, s := range samples {
		label, ok := uniqueLabel(seen, s.sensor, s.sensorID)
		if !ok {
			c.logger.Warn("skipping humidity reading with duplicate labels", "sensor", s.sensor, "sensor_id", s.sensorID, "chassis", chassisID, "target", c.target)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.humidityDesc, prometheus.GaugeValue, s.percent, chassisID, label, s.context)
		for _, th := range s.thresholds {
			ch <- prometheus.MustNewConstMetric(c.humidityThresholdDesc, prometheus.GaugeValue, th.value, chassisID, label, s.context, th.level)
		}
	}
}

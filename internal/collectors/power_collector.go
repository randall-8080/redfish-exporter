package collectors

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

type PowerCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors
	powerStateDesc            *prometheus.Desc
	chassisHealthDesc         *prometheus.Desc
	chassisHealthRollupDesc   *prometheus.Desc
	powerCapacityDesc         *prometheus.Desc
	powerConsumedDesc         *prometheus.Desc
	powerAverageConsumedDesc  *prometheus.Desc
	powerMaxConsumedDesc      *prometheus.Desc
	powerMinConsumedDesc      *prometheus.Desc
	powerControlHealthDesc    *prometheus.Desc
	powerRedundancyHealthDesc *prometheus.Desc
	powerSuppliesHealthDesc   *prometheus.Desc

	// descriptors — modern resources
	powerSubsystemHealthDesc *prometheus.Desc
	psuInputWattsDesc        *prometheus.Desc
	psuOutputWattsDesc       *prometheus.Desc
	psuInputVoltsDesc        *prometheus.Desc
	psuInputAmpsDesc         *prometheus.Desc
	psuTemperatureDesc       *prometheus.Desc
	sensorPowerDesc          *prometheus.Desc
	sensorVoltageDesc        *prometheus.Desc
	sensorCurrentDesc        *prometheus.Desc
	sensorPowerThreshold     *prometheus.Desc
	sensorVoltageThreshold   *prometheus.Desc
	sensorCurrentThreshold   *prometheus.Desc
}

// NewPowerCollector creates a new PowerCollector instance.
// It collects power metrics from the legacy Power resource, the modern
// PowerSubsystem tree, chassis Sensors, and EnvironmentMetrics.
func NewPowerCollector(target string, client *gofish.APIClient, logger *slog.Logger) *PowerCollector {
	powerBaseLabels := []string{"chassis_id"}
	psuLabels := []string{"chassis_id", "name", "member_id"}
	sensorLabels := []string{"chassis_id", "sensor", "physical_context"}
	sensorThresholdLabels := []string{"chassis_id", "sensor", "physical_context", "level"}

	return &PowerCollector{
		target: target,
		client: client,
		logger: logger,
		powerStateDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "state"),
			"Power state of the overall chassis (1=On, 0=Off)",
			powerBaseLabels,
			prometheus.Labels{"target": target},
		),
		chassisHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "chassis", "health"),
			"Health of the chassis (1=OK, 0=not OK)",
			append(powerBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		chassisHealthRollupDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "chassis", "health_rollup"),
			"Rollup health of the chassis including all subordinate resources (1=OK, 0=not OK)",
			append(powerBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		powerCapacityDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "capacity_watts"),
			"Power capacity of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		powerConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "consumed_watts"),
			"Power consumed currently of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		powerAverageConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "average_consumed_watts"),
			"Power average consumed of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		powerMaxConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "max_consumed_watts"),
			"Power max consumed of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		powerMinConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "min_consumed_watts"),
			"Power min consumed of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		powerControlHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "control_health"),
			"Health of a power control domain (1=OK, 0=not OK)",
			append(powerBaseLabels, "name", "member_id", "health"),
			prometheus.Labels{"target": target},
		),
		powerRedundancyHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "redundancy_health"),
			"Health status of the power redundancy subsystem (1=OK, 0=not OK)",
			append(powerBaseLabels, "name", "member_id", "health"),
			prometheus.Labels{"target": target},
		),
		powerSuppliesHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "supply_health"),
			"Health status of a power supply (1=OK, 0=not OK)",
			append(psuLabels, "health"),
			prometheus.Labels{"target": target},
		),
		powerSubsystemHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_subsystem", "health"),
			"Health of the power subsystem (1=OK, 0=not OK)",
			append(powerBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		psuInputWattsDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_supply", "input_watts"),
			"Power supply input power in Watts",
			psuLabels,
			prometheus.Labels{"target": target},
		),
		psuOutputWattsDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_supply", "output_watts"),
			"Power supply output power in Watts",
			psuLabels,
			prometheus.Labels{"target": target},
		),
		psuInputVoltsDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_supply", "input_volts"),
			"Power supply input voltage in Volts",
			psuLabels,
			prometheus.Labels{"target": target},
		),
		psuInputAmpsDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_supply", "input_amps"),
			"Power supply input current in Amperes",
			psuLabels,
			prometheus.Labels{"target": target},
		),
		psuTemperatureDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_supply", "temperature_celsius"),
			"Power supply temperature in Celsius",
			psuLabels,
			prometheus.Labels{"target": target},
		),
		sensorPowerDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "sensor", "power_watts"),
			"Power sensor reading in Watts",
			sensorLabels,
			prometheus.Labels{"target": target},
		),
		sensorVoltageDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "sensor", "voltage_volts"),
			"Voltage sensor reading in Volts",
			sensorLabels,
			prometheus.Labels{"target": target},
		),
		sensorCurrentDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "sensor", "current_amps"),
			"Current sensor reading in Amperes",
			sensorLabels,
			prometheus.Labels{"target": target},
		),
		sensorPowerThreshold: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "sensor", "power_threshold_watts"),
			"Power sensor threshold in Watts",
			sensorThresholdLabels,
			prometheus.Labels{"target": target},
		),
		sensorVoltageThreshold: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "sensor", "voltage_threshold_volts"),
			"Voltage sensor threshold in Volts",
			sensorThresholdLabels,
			prometheus.Labels{"target": target},
		),
		sensorCurrentThreshold: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "sensor", "current_threshold_amps"),
			"Current sensor threshold in Amperes",
			sensorThresholdLabels,
			prometheus.Labels{"target": target},
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected by this Collector.
func (c *PowerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.powerStateDesc
	ch <- c.chassisHealthDesc
	ch <- c.chassisHealthRollupDesc
	ch <- c.powerCapacityDesc
	ch <- c.powerConsumedDesc
	ch <- c.powerAverageConsumedDesc
	ch <- c.powerMaxConsumedDesc
	ch <- c.powerMinConsumedDesc
	ch <- c.powerControlHealthDesc
	ch <- c.powerRedundancyHealthDesc
	ch <- c.powerSuppliesHealthDesc
	ch <- c.powerSubsystemHealthDesc
	ch <- c.psuInputWattsDesc
	ch <- c.psuOutputWattsDesc
	ch <- c.psuInputVoltsDesc
	ch <- c.psuInputAmpsDesc
	ch <- c.psuTemperatureDesc
	ch <- c.sensorPowerDesc
	ch <- c.sensorVoltageDesc
	ch <- c.sensorCurrentDesc
	ch <- c.sensorPowerThreshold
	ch <- c.sensorVoltageThreshold
	ch <- c.sensorCurrentThreshold
}

// Collect satisfies prometheus.Collector; use redfish_collector_success to detect failures.
// When run standalone it creates its own scrape context.
func (c *PowerCollector) Collect(ch chan<- prometheus.Metric) {
	_ = c.collect(newScrapeContext(c.client, c.logger, false), ch)
}

// collect walks all chassis and reports an error when any resource fetch
// failed; metrics collected from the remaining resources are still emitted.
func (c *PowerCollector) collect(ctx *scrapeContext, ch chan<- prometheus.Metric) error {
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

// collectChassis gathers every power source a chassis advertises and emits
// metrics per family using source ladders: modern sources first, legacy
// resources as fallback. Only the first source that yields a reading for a
// family is used, which prevents duplicate samples when the same physical
// measurement is represented in several resources.
func (c *PowerCollector) collectChassis(ctx *scrapeContext, chassis *schemas.Chassis, ch chan<- prometheus.Metric) error {
	var errs []error

	powerState := 0.0
	if chassis.PowerState == schemas.OnPowerState {
		powerState = 1.0
	}
	ch <- prometheus.MustNewConstMetric(c.powerStateDesc, prometheus.GaugeValue, powerState, chassis.ID)
	if chassis.Status.Health != "" {
		ch <- prometheus.MustNewConstMetric(
			c.chassisHealthDesc,
			prometheus.GaugeValue,
			healthToFloat(chassis.Status.Health),
			chassis.ID, string(chassis.Status.Health),
		)
	}
	if chassis.Status.HealthRollup != "" {
		ch <- prometheus.MustNewConstMetric(
			c.chassisHealthRollupDesc,
			prometheus.GaugeValue,
			healthToFloat(chassis.Status.HealthRollup),
			chassis.ID, string(chassis.Status.HealthRollup),
		)
	}

	legacyPower, err := chassis.Power()
	switch {
	case err == nil:
	case isNotFound(err):
		// The service links Power but does not implement it; treat the
		// deprecated resource as absent and rely on modern sources.
		c.logger.Debug("power resource not found", "chassis_id", chassis.ID, "target", c.target)
	default:
		c.logger.Error("failed to get power resource", "error", err, "chassis_id", chassis.ID, "target", c.target)
		errs = append(errs, err)
	}

	pss, err := ctx.PowerSubsystem(chassis)
	if err != nil {
		c.logger.Error("failed to get power subsystem", "error", err, "chassis_id", chassis.ID, "target", c.target)
		errs = append(errs, err)
	}

	envMetrics, err := ctx.EnvironmentMetrics(chassis)
	switch {
	case err == nil:
	case isNotFound(err):
		// EnvironmentMetrics is license-restricted on some vendors; treat it
		// as a capability gap.
		c.logger.Debug("environment metrics not found", "chassis_id", chassis.ID, "target", c.target)
	default:
		c.logger.Error("failed to get environment metrics", "error", err, "chassis_id", chassis.ID, "target", c.target)
		errs = append(errs, err)
	}

	sensors, err := ctx.Sensors(chassis)
	if err != nil {
		c.logger.Error("failed to get sensors", "error", err, "chassis_id", chassis.ID, "target", c.target)
		errs = append(errs, err)
	}

	// Power subsystem health. Systems without redundant supplies (e.g.
	// wall-powered Supermicro boards) report State=Absent with no health.
	if pss.subsystem != nil && pss.subsystem.Status.Health != "" {
		ch <- prometheus.MustNewConstMetric(
			c.powerSubsystemHealthDesc,
			prometheus.GaugeValue,
			healthToFloat(pss.subsystem.Status.Health),
			chassis.ID, string(pss.subsystem.Status.Health),
		)
	}

	// Power capacity: subsystem capacity → legacy power control zones. The
	// subsystem capacity is not reported when the subsystem is absent.
	capacityEmitted := false
	if pss.subsystem != nil && pss.subsystem.Status.State != schemas.AbsentState && pss.subsystem.CapacityWatts != nil {
		ch <- prometheus.MustNewConstMetric(
			c.powerCapacityDesc,
			prometheus.GaugeValue,
			*pss.subsystem.CapacityWatts,
			chassis.ID, "PowerSubsystem", "",
		)
		capacityEmitted = true
	}

	// Power consumption: environment metrics excerpt → legacy power control
	// zones. The excerpt sensor is suppressed from the generic sensor family
	// when it is emitted here.
	consumedEmitted := false
	envPowerURI := ""
	if envMetrics != nil && envMetrics.PowerWatts.Reading != nil {
		ch <- prometheus.MustNewConstMetric(
			c.powerConsumedDesc,
			prometheus.GaugeValue,
			*envMetrics.PowerWatts.Reading,
			chassis.ID, "EnvironmentMetrics", "",
		)
		consumedEmitted = true
		envPowerURI = envMetrics.PowerWatts.DataSourceURI
	}

	// Power control zones: legacy only. Capacity and consumption fall back
	// to the zones when the modern sources yielded nothing.
	if legacyPower != nil {
		for _, pc := range legacyPower.PowerControl {
			if pc.Status.Health != "" {
				ch <- prometheus.MustNewConstMetric(
					c.powerControlHealthDesc,
					prometheus.GaugeValue,
					healthToFloat(pc.Status.Health),
					chassis.ID, pc.Name, pc.MemberID, string(pc.Status.Health),
				)
			}
			if !capacityEmitted && pc.PowerCapacityWatts != nil {
				ch <- prometheus.MustNewConstMetric(
					c.powerCapacityDesc,
					prometheus.GaugeValue,
					float64(*pc.PowerCapacityWatts),
					chassis.ID, pc.Name, pc.MemberID,
				)
			}
			if !consumedEmitted && pc.PowerConsumedWatts != nil {
				ch <- prometheus.MustNewConstMetric(
					c.powerConsumedDesc,
					prometheus.GaugeValue,
					float64(*pc.PowerConsumedWatts),
					chassis.ID, pc.Name, pc.MemberID,
				)
			}
			if pc.PowerMetrics.AverageConsumedWatts != nil {
				ch <- prometheus.MustNewConstMetric(
					c.powerAverageConsumedDesc,
					prometheus.GaugeValue,
					float64(*pc.PowerMetrics.AverageConsumedWatts),
					chassis.ID, pc.Name, pc.MemberID,
				)
			}
			if pc.PowerMetrics.MaxConsumedWatts != nil {
				ch <- prometheus.MustNewConstMetric(
					c.powerMaxConsumedDesc,
					prometheus.GaugeValue,
					float64(*pc.PowerMetrics.MaxConsumedWatts),
					chassis.ID, pc.Name, pc.MemberID,
				)
			}
			if pc.PowerMetrics.MinConsumedWatts != nil {
				ch <- prometheus.MustNewConstMetric(
					c.powerMinConsumedDesc,
					prometheus.GaugeValue,
					float64(*pc.PowerMetrics.MinConsumedWatts),
					chassis.ID, pc.Name, pc.MemberID,
				)
			}
		}
	}

	// Redundancy: subsystem redundancy groups → legacy redundancy groups.
	redundancyEmitted := false
	if pss.subsystem != nil {
		seenGroups := make(map[string]string)
		for i, rg := range pss.subsystem.PowerSupplyRedundancy {
			if rg.Status.Health == "" {
				continue
			}
			name := rg.GroupName
			if name == "" {
				name = string(rg.RedundancyType)
			}
			if name == "" {
				name = "PowerSupplyRedundancy"
			}
			// Groups without a name fall back to their type; disambiguate
			// same-typed groups so they cannot produce duplicate series.
			name, ok := uniqueLabel(seenGroups, name, strconv.Itoa(i))
			if !ok {
				c.logger.Warn("skipping redundancy group with duplicate labels", "chassis", chassis.ID, "target", c.target)
				continue
			}
			ch <- prometheus.MustNewConstMetric(
				c.powerRedundancyHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(rg.Status.Health),
				chassis.ID, name, "", string(rg.Status.Health),
			)
			redundancyEmitted = true
		}
	}
	if !redundancyEmitted && legacyPower != nil {
		for _, redundancy := range legacyPower.Redundancy {
			if redundancy.Name == "" {
				c.logger.Debug("skipping unnamed redundancy entry", "chassis", chassis.ID, "target", c.target)
				continue
			}
			ch <- prometheus.MustNewConstMetric(
				c.powerRedundancyHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(redundancy.Status.Health),
				chassis.ID, redundancy.Name, redundancy.MemberID, string(redundancy.Status.Health),
			)
		}
	}

	// Power supply health: subsystem supplies → legacy supplies. Modern
	// supplies identify themselves by Id; the legacy array members by
	// MemberId.
	supplyHealthEmitted := false
	for _, psu := range pss.psus {
		if psu.Status.State == schemas.AbsentState {
			continue
		}
		ch <- prometheus.MustNewConstMetric(
			c.powerSuppliesHealthDesc,
			prometheus.GaugeValue,
			healthToFloat(psu.Status.Health),
			chassis.ID, psuName(psu), psu.ID, string(psu.Status.Health),
		)
		supplyHealthEmitted = true
	}
	if !supplyHealthEmitted && legacyPower != nil {
		for _, ps := range legacyPower.PowerSupplies {
			if ps.Name == "" {
				c.logger.Debug("skipping unnamed power supply", "chassis", chassis.ID, "target", c.target)
				continue
			}
			ch <- prometheus.MustNewConstMetric(
				c.powerSuppliesHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(ps.Status.Health),
				chassis.ID, ps.Name, ps.MemberID, string(ps.Status.Health),
			)
		}
	}

	// Power supply readings: PSU metrics excerpts → legacy supply fields,
	// per reading kind. The legacy fields are used for a kind only when the
	// metrics resources emitted nothing for it.
	var psuInputWatts, psuOutputWatts, psuInputVolts, psuInputAmps, psuTemps []psuSample
	consumedPower := make(map[string]bool)
	consumedVoltage := make(map[string]bool)
	consumedCurrent := make(map[string]bool)
	for _, psu := range pss.psus {
		if psu.Status.State == schemas.AbsentState {
			continue
		}
		m := pss.metrics[psu.ODataID]
		if m == nil {
			continue
		}
		name, memberID := psuName(psu), psu.ID
		if r := m.InputPowerWatts; r.Reading != nil {
			psuInputWatts = append(psuInputWatts, psuSample{name: name, memberID: memberID, value: *r.Reading})
			if r.DataSourceURI != "" {
				consumedPower[r.DataSourceURI] = true
			}
		}
		if r := m.OutputPowerWatts; r.Reading != nil {
			psuOutputWatts = append(psuOutputWatts, psuSample{name: name, memberID: memberID, value: *r.Reading})
			if r.DataSourceURI != "" {
				consumedPower[r.DataSourceURI] = true
			}
		}
		if r := m.InputVoltage; r.Reading != nil {
			psuInputVolts = append(psuInputVolts, psuSample{name: name, memberID: memberID, value: *r.Reading})
			if r.DataSourceURI != "" {
				consumedVoltage[r.DataSourceURI] = true
			}
		}
		if r := m.InputCurrentAmps; r.Reading != nil {
			psuInputAmps = append(psuInputAmps, psuSample{name: name, memberID: memberID, value: *r.Reading})
			if r.DataSourceURI != "" {
				consumedCurrent[r.DataSourceURI] = true
			}
		}
		if r := m.TemperatureCelsius; r.Reading != nil {
			psuTemps = append(psuTemps, psuSample{name: name, memberID: memberID, value: *r.Reading})
		}
	}
	if legacyPower != nil {
		fallbackInputWatts := len(psuInputWatts) == 0
		fallbackOutputWatts := len(psuOutputWatts) == 0
		fallbackInputVolts := len(psuInputVolts) == 0
		for _, ps := range legacyPower.PowerSupplies {
			if ps.Status.State == schemas.AbsentState || ps.Name == "" {
				continue
			}
			if fallbackInputWatts && ps.PowerInputWatts != nil {
				psuInputWatts = append(psuInputWatts, psuSample{name: ps.Name, memberID: ps.MemberID, value: float64(*ps.PowerInputWatts)})
			}
			if fallbackOutputWatts && ps.PowerOutputWatts != nil {
				psuOutputWatts = append(psuOutputWatts, psuSample{name: ps.Name, memberID: ps.MemberID, value: float64(*ps.PowerOutputWatts)})
			}
			if fallbackInputVolts && ps.LineInputVoltage != nil {
				psuInputVolts = append(psuInputVolts, psuSample{name: ps.Name, memberID: ps.MemberID, value: float64(*ps.LineInputVoltage)})
			}
		}
	}
	emitPSUSamples(ch, c.psuInputWattsDesc, chassis.ID, psuInputWatts)
	emitPSUSamples(ch, c.psuOutputWattsDesc, chassis.ID, psuOutputWatts)
	emitPSUSamples(ch, c.psuInputVoltsDesc, chassis.ID, psuInputVolts)
	emitPSUSamples(ch, c.psuInputAmpsDesc, chassis.ID, psuInputAmps)
	emitPSUSamples(ch, c.psuTemperatureDesc, chassis.ID, psuTemps)
	if envPowerURI != "" {
		consumedPower[envPowerURI] = true
	}

	// Generic sensor readings: power, voltage, and current sensors that no
	// emitted excerpt has claimed through its DataSourceUri.
	var powerSensors, voltageSensors, currentSensors []*schemas.Sensor
	for _, s := range sensors {
		switch s.ReadingType {
		case schemas.PowerReadingType:
			powerSensors = append(powerSensors, s)
		case schemas.VoltageReadingType:
			voltageSensors = append(voltageSensors, s)
		case schemas.CurrentReadingType:
			currentSensors = append(currentSensors, s)
		}
	}
	c.emitSensorReadings(chassis.ID, powerSensors, consumedPower, c.sensorPowerDesc, c.sensorPowerThreshold, ch)
	voltageEmitted := c.emitSensorReadings(chassis.ID, voltageSensors, consumedVoltage, c.sensorVoltageDesc, c.sensorVoltageThreshold, ch)
	c.emitSensorReadings(chassis.ID, currentSensors, consumedCurrent, c.sensorCurrentDesc, c.sensorCurrentThreshold, ch)

	// Voltage falls back to the legacy Voltages array when no unconsumed
	// voltage sensor yielded a reading.
	if voltageEmitted == 0 && legacyPower != nil {
		seen := make(map[string]string)
		for _, v := range legacyPower.Voltages {
			if v.Status.State == schemas.AbsentState || v.ReadingVolts == nil {
				continue
			}
			name := v.Name
			if name == "" {
				name = v.MemberID
			}
			if name == "" {
				continue
			}
			label, ok := uniqueLabel(seen, name, v.MemberID)
			if !ok {
				c.logger.Warn("skipping voltage reading with duplicate labels", "name", name, "member_id", v.MemberID, "chassis", chassis.ID, "target", c.target)
				continue
			}
			ch <- prometheus.MustNewConstMetric(c.sensorVoltageDesc, prometheus.GaugeValue, float64(*v.ReadingVolts), chassis.ID, label, string(v.PhysicalContext))
		}
	}

	return errors.Join(errs...)
}

// psuName returns the display name of a power supply resource, falling back
// to its stable ID when the service did not provide a name.
func psuName(psu *schemas.PowerSupplyUnit) string {
	if psu.Name != "" {
		return psu.Name
	}
	return psu.ID
}

// psuSample is a power supply reading from a metrics excerpt or a legacy
// supply field.
type psuSample struct {
	name     string
	memberID string
	value    float64
}

// emitPSUSamples emits a list of power supply readings.
func emitPSUSamples(ch chan<- prometheus.Metric, desc *prometheus.Desc, chassisID string, samples []psuSample) {
	for _, s := range samples {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, s.value, chassisID, s.name, s.memberID)
	}
}

// emitSensorReadings emits generic sensor readings with their thresholds,
// skipping sensors claimed by an emitted excerpt, and returns the number of
// readings emitted.
func (c *PowerCollector) emitSensorReadings(chassisID string, sensors []*schemas.Sensor, consumed map[string]bool, desc, thresholdDesc *prometheus.Desc, ch chan<- prometheus.Metric) int {
	seen := make(map[string]string)
	emitted := 0
	for _, s := range sensors {
		if s.Reading == nil || s.Status.State == schemas.DisabledState || s.Status.State == schemas.AbsentState {
			continue
		}
		if consumed[s.ODataID] {
			continue
		}
		label, ok := uniqueLabel(seen, sensorName(s), s.ID)
		if !ok {
			c.logger.Warn("skipping sensor reading with duplicate labels", "sensor", s.Name, "sensor_id", s.ID, "chassis", chassisID, "target", c.target)
			continue
		}
		context := string(s.PhysicalContext)
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, *s.Reading, chassisID, label, context)
		emitted++
		for _, th := range thresholdValues(s.Thresholds) {
			ch <- prometheus.MustNewConstMetric(thresholdDesc, prometheus.GaugeValue, th.value, chassisID, label, context, th.level)
		}
	}
	return emitted
}

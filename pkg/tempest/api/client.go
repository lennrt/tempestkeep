// Package api provides a bounded WeatherFlow Tempest REST client.
//
// Client methods accept a context, retry transient failures, and redact the
// token from errors. DeviceObservations performs one request for at most five
// days. The collect package owns multi-request backfills.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/lennrt/tempestkeep/pkg/tempest/model"
)

const base = "https://swd.weatherflow.com/swd/rest"

const (
	defaultCacheTTL       = 5 * time.Minute
	defaultRequestTimeout = 30 * time.Second
	defaultMaxAttempts    = 4
	defaultRetryBaseWait  = time.Second
	defaultRetryMaxWait   = 30 * time.Second
	maxResponseSize       = 8 << 20
	maxCacheEntrySize     = 2 << 20
	maxCacheEntries       = 32
	maxTokenBytes         = 4096
	maxStations           = 256
	maxDevicesPerStation  = 256
	maxStationObs         = 1000
	maxDeviceObs          = 10_000
	maxDailyForecasts     = 32
	maxHourlyForecasts    = 24 * 14
	maxTextBytes          = 1024
)

// Stable errors let callers classify failures without parsing text.
var (
	ErrInvalidArgument   = errors.New("invalid argument")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrNoDevice          = errors.New("no Tempest device")
	ErrNoObservation     = errors.New("no observation")
	ErrResponseTooLarge  = errors.New("response too large")
	ErrMalformedResponse = errors.New("malformed response")
	ErrTransport         = errors.New("HTTP transport failure")
)

// HTTPError reports a non-success response. Operation never contains an ID,
// URL, token, query, or response body.
type HTTPError struct {
	Operation  string
	StatusCode int
	Retryable  bool
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s request failed: status %d", e.Operation, e.StatusCode)
}

func operation(path string) string {
	switch {
	case strings.HasPrefix(path, "observations/device/"):
		return "device observations"
	case strings.HasPrefix(path, "observations/station/"):
		return "station observations"
	case path == "better_forecast":
		return "forecast"
	case path == "stations":
		return "stations"
	default:
		return "API"
	}
}

// RetryPolicy bounds retry attempts and exponential backoff.
type RetryPolicy struct {
	MaxAttempts int
	BaseWait    time.Duration
	MaxWait     time.Duration
}

type clientConfig struct {
	baseURL        *url.URL
	httpClient     *http.Client
	cacheTTL       time.Duration
	requestTimeout time.Duration
	retry          RetryPolicy
}

// Option configures a Client without performing I/O.
type Option func(*clientConfig) error

// WithBaseURL sets the REST endpoint. The URL must use HTTP or HTTPS and must
// not contain credentials, a query, or a fragment.
func WithBaseURL(raw string) Option {
	return func(cfg *clientConfig) error {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("%w: API base URL must be an HTTP(S) origin or path without credentials, query, or fragment", ErrInvalidArgument)
		}
		u.Path = strings.TrimRight(u.Path, "/")
		cfg.baseURL = u
		return nil
	}
}

// WithHTTPClient supplies a borrowed HTTP client. The transport receives the
// token in each request URL. The caller must trust it and must not mutate it.
func WithHTTPClient(client *http.Client) Option {
	return func(cfg *clientConfig) error {
		if client == nil {
			return fmt.Errorf("%w: HTTP client is nil", ErrInvalidArgument)
		}
		cfg.httpClient = client
		return nil
	}
}

// WithCacheTTL sets the in-memory cache lifetime. Zero disables caching.
func WithCacheTTL(ttl time.Duration) Option {
	return func(cfg *clientConfig) error {
		if ttl < 0 || ttl > 24*time.Hour {
			return fmt.Errorf("%w: cache TTL must be between 0 and 24h", ErrInvalidArgument)
		}
		cfg.cacheTTL = ttl
		return nil
	}
}

// WithRequestTimeout bounds each HTTP attempt. The caller's earlier context
// deadline still wins.
func WithRequestTimeout(timeout time.Duration) Option {
	return func(cfg *clientConfig) error {
		if timeout <= 0 || timeout > 5*time.Minute {
			return fmt.Errorf("%w: request timeout must be greater than 0 and at most 5m", ErrInvalidArgument)
		}
		cfg.requestTimeout = timeout
		return nil
	}
}

// WithRetryPolicy sets a bounded retry policy. MaxAttempts includes the first
// request. BaseWait and MaxWait must be positive.
func WithRetryPolicy(policy RetryPolicy) Option {
	return func(cfg *clientConfig) error {
		if policy.MaxAttempts < 1 || policy.MaxAttempts > 10 || policy.BaseWait <= 0 ||
			policy.MaxWait < policy.BaseWait || policy.MaxWait > 5*time.Minute {
			return fmt.Errorf("%w: retry policy requires 1..10 attempts and 0 < base wait <= max wait <= 5m", ErrInvalidArgument)
		}
		cfg.retry = policy
		return nil
	}
}

// Client talks to the Tempest REST API with a Personal Access Token. It is safe
// for concurrent use.
type Client struct {
	token          string
	baseURL        *url.URL
	http           *http.Client
	requestTimeout time.Duration
	retry          RetryPolicy

	ttl   time.Duration
	mu    sync.Mutex
	cache map[string]cachedBody
}

type cachedBody struct {
	body []byte
	exp  time.Time
}

// New validates token and options and returns a configured Client. It does not
// read the environment, access the filesystem, start a goroutine, or contact
// the network.
func New(token string, options ...Option) (*Client, error) {
	if token == "" || len(token) > maxTokenBytes || strings.TrimSpace(token) != token ||
		strings.IndexFunc(token, unicode.IsControl) >= 0 {
		return nil, fmt.Errorf("%w: token must contain 1..%d non-control bytes without surrounding whitespace", ErrInvalidArgument, maxTokenBytes)
	}
	baseURL, _ := url.Parse(base)
	cfg := clientConfig{
		baseURL:        baseURL,
		httpClient:     &http.Client{},
		cacheTTL:       defaultCacheTTL,
		requestTimeout: defaultRequestTimeout,
		retry: RetryPolicy{
			MaxAttempts: defaultMaxAttempts,
			BaseWait:    defaultRetryBaseWait,
			MaxWait:     defaultRetryMaxWait,
		},
	}
	for i, option := range options {
		if option == nil {
			return nil, fmt.Errorf("%w: option %d is nil", ErrInvalidArgument, i)
		}
		if err := option(&cfg); err != nil {
			return nil, fmt.Errorf("option %d: %w", i, err)
		}
	}
	return &Client{
		token:          token,
		baseURL:        cfg.baseURL,
		http:           cfg.httpClient,
		requestTimeout: cfg.requestTimeout,
		retry:          cfg.retry,
		ttl:            cfg.cacheTTL,
		cache:          make(map[string]cachedBody),
	}, nil
}

func (c *Client) cacheGet(key string) ([]byte, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Now().After(e.exp) {
		delete(c.cache, key)
		return nil, false
	}
	return bytes.Clone(e.body), true
}

func (c *Client) cachePut(key string, body []byte) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(body) > maxCacheEntrySize {
		return
	}
	// Remove expired entries, then evict the entry nearest expiry when the hard
	// cap is reached. This bounds retained response data to 64 MiB or less.
	now := time.Now()
	for k, e := range c.cache {
		if now.After(e.exp) {
			delete(c.cache, k)
		}
	}
	if _, exists := c.cache[key]; !exists && len(c.cache) >= maxCacheEntries {
		var oldestKey string
		var oldest time.Time
		for k, e := range c.cache {
			if oldestKey == "" || e.exp.Before(oldest) {
				oldestKey, oldest = k, e.exp
			}
		}
		delete(c.cache, oldestKey)
	}
	c.cache[key] = cachedBody{body: bytes.Clone(body), exp: now.Add(c.ttl)}
}

// cacheable reports whether a path's response is worth caching. Historical
// device windows are excluded: a backfill fetches each window exactly once, so
// caching them would only accumulate megabytes of dead JSON per 5-day chunk.
func cacheable(path string) bool {
	return !strings.HasPrefix(path, "observations/device/")
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	if c == nil || c.baseURL == nil || c.http == nil || c.token == "" {
		return fmt.Errorf("%w: API client is nil or not initialized", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	key := path + "?" + q.Encode() // token not yet added -> stable, token-independent key
	if cacheable(path) {
		if b, ok := c.cacheGet(key); ok {
			if err := json.Unmarshal(b, out); err != nil {
				return fmt.Errorf("%w: cached %s response is invalid", ErrMalformedResponse, operation(path))
			}
			return nil
		}
	}

	q = q.Clone()
	q.Set("token", c.token)
	u := c.baseURL.JoinPath(path)
	u.RawQuery = q.Encode()
	body, err := c.fetch(ctx, path, u.String())
	if err != nil {
		return err
	}
	if cacheable(path) {
		c.cachePut(key, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%w: %s response is invalid", ErrMalformedResponse, operation(path))
	}
	return nil
}

// fetch GETs u, retrying transient failures with exponential backoff (see
// maxAttempts). It gives up early when the context is done, so cancellation is
// never stalled by a backoff sleep.
func (c *Client) fetch(ctx context.Context, path, u string) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		body, retryable, retryAfter, err := c.fetchOnce(ctx, path, u)
		if err == nil {
			return body, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !retryable || attempt >= c.retry.MaxAttempts {
			return nil, err
		}
		wait := min(max(retryAfter, c.retry.BaseWait<<(attempt-1)), c.retry.MaxWait)
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}

// fetchOnce performs a single GET. retryable marks failures worth another
// attempt: network errors and 429/5xx statuses, with retryAfter carrying the
// server's Retry-After hint when it sends one. Anything embedding the request
// URL is passed through redactToken so the secret never reaches an error string.
func (c *Client) fetchOnce(ctx context.Context, path, u string) (body []byte, retryable bool, retryAfter time.Duration, err error) {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, 0, c.redactToken(err)
	}
	req.Header.Set("User-Agent", "tempestkeep (github.com/lennrt/tempestkeep)")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, 0, c.redactToken(err)
	}
	defer func() {
		_, drainErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		err = errors.Join(err, c.redactToken(drainErr), c.redactToken(resp.Body.Close()))
	}()
	switch {
	case resp.StatusCode == http.StatusOK:
		if resp.ContentLength > maxResponseSize {
			return nil, false, 0, fmt.Errorf("%w: %s response exceeds %d MiB", ErrResponseTooLarge, operation(path), maxResponseSize>>20)
		}
		if contentType := resp.Header.Get("Content-Type"); contentType != "" {
			mediaType, _, parseErr := mime.ParseMediaType(contentType)
			if parseErr != nil || mediaType != "application/json" {
				return nil, false, 0, fmt.Errorf("%w: %s response is not JSON", ErrMalformedResponse, operation(path))
			}
		}
		body, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
		if err != nil {
			return nil, true, 0, c.redactToken(err)
		}
		if len(body) > maxResponseSize {
			return nil, false, 0, fmt.Errorf("%w: %s response exceeds %d MiB", ErrResponseTooLarge, operation(path), maxResponseSize>>20)
		}
		return body, false, 0, nil
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, false, 0, fmt.Errorf("%w: check the Tempest token", ErrUnauthorized)
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		return nil, true, retryAfter, &HTTPError{Operation: operation(path), StatusCode: resp.StatusCode, Retryable: true}
	default:
		return nil, false, 0, &HTTPError{Operation: operation(path), StatusCode: resp.StatusCode}
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

// redactToken replaces transport details with a stable error. Transport errors
// can contain the token, station IDs, device IDs, and local endpoint names.
func (c *Client) redactToken(err error) error {
	if err == nil {
		return nil
	}
	return ErrTransport
}

// Device is one sensor device attached to a station. device_type "ST" is a
// Tempest; "AR"/"SK" are the older Air/Sky units.
type Device struct {
	DeviceID     int    `json:"device_id"`
	DeviceType   string `json:"device_type"`
	SerialNumber string `json:"serial_number"`
}

// StationMeta carries extra station metadata (we use elevation).
type StationMeta struct {
	Elevation float64 `json:"elevation"`
}

// Station is a station visible to the token, with its location and devices.
type Station struct {
	StationID   int         `json:"station_id"`
	Name        string      `json:"name"`
	Latitude    float64     `json:"latitude"`
	Longitude   float64     `json:"longitude"`
	Timezone    string      `json:"timezone"`
	StationMeta StationMeta `json:"station_meta"`
	Devices     []Device    `json:"devices"`
}

// Stations lists the stations (and their devices) this token can access.
func (c *Client) Stations(ctx context.Context) ([]Station, error) {
	var out struct {
		Stations []Station `json:"stations"`
	}
	if err := c.get(ctx, "stations", url.Values{}, &out); err != nil {
		return nil, err
	}
	if len(out.Stations) > maxStations {
		return nil, fmt.Errorf("%w: stations response exceeds %d stations", ErrMalformedResponse, maxStations)
	}
	for index := range out.Stations {
		if err := validateStation(out.Stations[index]); err != nil {
			return nil, fmt.Errorf("%w: station %d: %w", ErrMalformedResponse, index, err)
		}
	}
	return out.Stations, nil
}

func validateStation(station Station) error {
	if station.StationID <= 0 || !finiteInRange(station.Latitude, -90, 90) ||
		!finiteInRange(station.Longitude, -180, 180) ||
		!boundedText(station.Name) || !boundedText(station.Timezone) ||
		!finiteInRange(station.StationMeta.Elevation, -500, 10_000) {
		return errors.New("station fields are outside accepted bounds")
	}
	if len(station.Devices) > maxDevicesPerStation {
		return fmt.Errorf("device list exceeds %d entries", maxDevicesPerStation)
	}
	for _, device := range station.Devices {
		if device.DeviceID <= 0 || !boundedText(device.DeviceType) || !boundedText(device.SerialNumber) {
			return errors.New("device fields are outside accepted bounds")
		}
	}
	return nil
}

func boundedText(value string) bool {
	return len(value) <= maxTextBytes && strings.IndexByte(value, 0) < 0
}

func finiteInRange(value, minimum, maximum float64) bool {
	return value >= minimum && value <= maximum
}

// PickTempestDevice selects the station and device id to archive from a station
// list: the first Tempest ("ST") device found, or, if the token owns only older
// Air/Sky hardware, the first device of the first station, so a non-Tempest
// account still works. It reports false only when no station has any device.
// The returned station and its Devices slice are owned copies. This is the pure
// selection rule shared by the CLI and MCP server; FindTempestDevice adds the
// live lookup.
func PickTempestDevice(stations []Station) (*Station, int, bool) {
	for i := range stations {
		for _, d := range stations[i].Devices {
			if d.DeviceType == "ST" { // ST = Tempest
				station := cloneStation(stations[i])
				return &station, d.DeviceID, true
			}
		}
	}
	for i := range stations {
		if len(stations[i].Devices) > 0 {
			station := cloneStation(stations[i])
			return &station, station.Devices[0].DeviceID, true
		}
	}
	return nil, 0, false
}

func cloneStation(station Station) Station {
	station.Devices = append([]Device(nil), station.Devices...)
	return station
}

// FindTempestDevice looks up the token's stations and returns the station and
// device id to archive (see PickTempestDevice). It errors if the token can see
// no device at all.
func (c *Client) FindTempestDevice(ctx context.Context) (*Station, int, error) {
	stations, err := c.Stations(ctx)
	if err != nil {
		return nil, 0, err
	}
	s, deviceID, ok := PickTempestDevice(stations)
	if !ok {
		return nil, 0, fmt.Errorf("%w for this token (use list-devices to inspect)", ErrNoDevice)
	}
	return s, deviceID, nil
}

// siUnits returns the query params that force SI units in a response, so results
// don't depend on the account's display settings.
func siUnits() url.Values {
	q := url.Values{}
	q.Set("units_temp", "c")
	q.Set("units_wind", "mps")
	q.Set("units_pressure", "mb")
	q.Set("units_precip", "mm")
	q.Set("units_distance", "km")
	return q
}

// StationObs is a rolled-up station observation. WeatherFlow already computes
// derived values (feels_like, dew_point). All sensor fields are pointers because
// any of them may be absent; values are SI (see LatestStationObs).
type StationObs struct {
	Timestamp           int64    `json:"timestamp"`
	AirTemperature      *float64 `json:"air_temperature"`
	RelativeHumidity    *float64 `json:"relative_humidity"`
	StationPressure     *float64 `json:"station_pressure"`
	SeaLevelPressure    *float64 `json:"sea_level_pressure"`
	WindAvg             *float64 `json:"wind_avg"`
	WindGust            *float64 `json:"wind_gust"`
	WindLull            *float64 `json:"wind_lull"`
	WindDirection       *float64 `json:"wind_direction"`
	UV                  *float64 `json:"uv"`
	SolarRadiation      *float64 `json:"solar_radiation"`
	Brightness          *float64 `json:"brightness"`
	FeelsLike           *float64 `json:"feels_like"`
	DewPoint            *float64 `json:"dew_point"`
	PrecipAccumLocalDay *float64 `json:"precip_accum_local_day"`
	LightningLast1hr    *int     `json:"lightning_strike_count_last_1hr"`
	LightningLast3hr    *int     `json:"lightning_strike_count_last_3hr"`
	LightningLastDist   *float64 `json:"lightning_strike_last_distance"`
}

// LatestStationObs fetches the most recent observation for a station (SI units).
func (c *Client) LatestStationObs(ctx context.Context, stationID int) (*StationObs, error) {
	if stationID <= 0 {
		return nil, fmt.Errorf("%w: station id must be positive", ErrInvalidArgument)
	}
	var out struct {
		Obs []StationObs `json:"obs"`
	}
	path := fmt.Sprintf("observations/station/%d", stationID)
	if err := c.get(ctx, path, siUnits(), &out); err != nil {
		return nil, err
	}
	if len(out.Obs) > maxStationObs {
		return nil, fmt.Errorf("%w: station observations response exceeds %d rows", ErrMalformedResponse, maxStationObs)
	}
	if len(out.Obs) == 0 {
		return nil, ErrNoObservation
	}
	for index := range out.Obs {
		if err := validateStationObs(out.Obs[index]); err != nil {
			return nil, fmt.Errorf("%w: station observation %d: %w", ErrMalformedResponse, index, err)
		}
	}
	return &out.Obs[len(out.Obs)-1], nil
}

func validateStationObs(observation StationObs) error {
	if observation.Timestamp <= 0 || observation.Timestamp > model.MaxEpochSeconds {
		return errors.New("timestamp is outside accepted bounds")
	}
	checks := []struct {
		value        *float64
		minimum, max float64
	}{
		{observation.AirTemperature, -150, 150},
		{observation.RelativeHumidity, 0, 100},
		{observation.StationPressure, 0, 2000},
		{observation.SeaLevelPressure, 0, 2000},
		{observation.WindAvg, 0, 200},
		{observation.WindGust, 0, 200},
		{observation.WindLull, 0, 200},
		{observation.WindDirection, 0, 360},
		{observation.UV, 0, 100},
		{observation.SolarRadiation, 0, 5000},
		{observation.Brightness, 0, 10_000_000},
		{observation.FeelsLike, -200, 200},
		{observation.DewPoint, -200, 200},
		{observation.PrecipAccumLocalDay, 0, 100_000},
		{observation.LightningLastDist, 0, 1000},
	}
	for _, check := range checks {
		if check.value != nil && !finiteInRange(*check.value, check.minimum, check.max) {
			return errors.New("sensor field is outside accepted bounds")
		}
	}
	for _, count := range []*int{observation.LightningLast1hr, observation.LightningLast3hr} {
		if count != nil && (*count < 0 || *count > 1_000_000) {
			return errors.New("lightning count is outside accepted bounds")
		}
	}
	return nil
}

// MaxDeviceWindow is the widest range the device-observations endpoint serves at
// full 1-minute resolution. WeatherFlow documents that one-minute data is
// available "for a time range that is five days or less," so DeviceObservations
// rejects wider windows and the collector walks history back in chunks no larger
// than this.
const MaxDeviceWindow = 5 * 24 * time.Hour

// DeviceObservations fetches the raw 1-minute observations a Tempest ("ST")
// device recorded in the inclusive epoch-seconds range [start, end]. This is the
// historical primitive behind the local archive: it returns the obs_st rows
// (SI units) for the collector to append.
//
// It makes a single request, so the window must be no wider than MaxDeviceWindow;
// chunking a long backfill is the caller's job. A malformed row fails the whole
// response so callers never mistake partial data for a complete time window.
func (c *Client) DeviceObservations(ctx context.Context, deviceID int, start, end int64) ([]model.DeviceObs, error) {
	if deviceID <= 0 {
		return nil, fmt.Errorf("%w: device id must be positive", ErrInvalidArgument)
	}
	if start < 0 || end < 0 || start > model.MaxEpochSeconds || end > model.MaxEpochSeconds {
		return nil, fmt.Errorf("%w: device observation timestamps must be in 0..%d", ErrInvalidArgument, model.MaxEpochSeconds)
	}
	if start > end {
		return nil, fmt.Errorf("%w: device observation range is not ordered", ErrInvalidArgument)
	}
	if windowSeconds := end - start; windowSeconds > int64(MaxDeviceWindow/time.Second) {
		return nil, fmt.Errorf("%w: device observation window exceeds %s", ErrInvalidArgument, MaxDeviceWindow)
	}
	q := siUnits()
	q.Set("time_start", strconv.FormatInt(start, 10))
	q.Set("time_end", strconv.FormatInt(end, 10))
	var out struct {
		Obs [][]*float64 `json:"obs"`
	}
	path := fmt.Sprintf("observations/device/%d", deviceID)
	if err := c.get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	if len(out.Obs) > maxDeviceObs {
		return nil, fmt.Errorf("%w: device observations response exceeds %d rows", ErrMalformedResponse, maxDeviceObs)
	}
	obs := make([]model.DeviceObs, 0, len(out.Obs))
	for index, row := range out.Obs {
		o, err := model.DeviceObsFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("%w: obs_st row %d: %w", ErrMalformedResponse, index, err)
		}
		obs = append(obs, o)
	}
	return obs, nil
}

// Forecast is the better_forecast response: current conditions plus hourly and
// daily forecasts. Fields are lenient (pointers / omitempty) so a changed or
// missing field degrades to empty rather than failing the decode.
type Forecast struct {
	CurrentConditions ForecastCurrent `json:"current_conditions"`
	Forecast          struct {
		Daily  []DailyForecast  `json:"daily"`
		Hourly []HourlyForecast `json:"hourly"`
	} `json:"forecast"`
}

// ForecastCurrent is the model's current conditions in a better_forecast reply.
type ForecastCurrent struct {
	Time             int64    `json:"time"`
	Conditions       string   `json:"conditions"`
	Icon             string   `json:"icon"`
	AirTemperature   *float64 `json:"air_temperature"`
	FeelsLike        *float64 `json:"feels_like"`
	RelativeHumidity *float64 `json:"relative_humidity"`
}

// DailyForecast is one day of the daily forecast (SI units).
type DailyForecast struct {
	DayStartLocal     int64    `json:"day_start_local"`
	Conditions        string   `json:"conditions"`
	Icon              string   `json:"icon"`
	Sunrise           int64    `json:"sunrise"`
	Sunset            int64    `json:"sunset"`
	AirTempHigh       *float64 `json:"air_temp_high"`
	AirTempLow        *float64 `json:"air_temp_low"`
	PrecipProbability *int     `json:"precip_probability"`
	PrecipType        string   `json:"precip_type"`
}

// HourlyForecast is one hour of the hourly forecast (SI units).
type HourlyForecast struct {
	Time              int64    `json:"time"`
	Conditions        string   `json:"conditions"`
	Icon              string   `json:"icon"`
	AirTemperature    *float64 `json:"air_temperature"`
	FeelsLike         *float64 `json:"feels_like"`
	RelativeHumidity  *float64 `json:"relative_humidity"`
	PrecipProbability *int     `json:"precip_probability"`
	WindAvg           *float64 `json:"wind_avg"`
	WindGust          *float64 `json:"wind_gust"`
	WindDirection     *float64 `json:"wind_direction"`
	UV                *float64 `json:"uv"`
}

// BetterForecast fetches the hourly + daily forecast for a station (SI units).
func (c *Client) BetterForecast(ctx context.Context, stationID int) (*Forecast, error) {
	if stationID <= 0 {
		return nil, fmt.Errorf("%w: station id must be positive", ErrInvalidArgument)
	}
	q := siUnits()
	q.Set("station_id", strconv.Itoa(stationID))
	var f Forecast
	if err := c.get(ctx, "better_forecast", q, &f); err != nil {
		return nil, err
	}
	if len(f.Forecast.Daily) > maxDailyForecasts || len(f.Forecast.Hourly) > maxHourlyForecasts {
		return nil, fmt.Errorf("%w: forecast response exceeds entry limits", ErrMalformedResponse)
	}
	if !boundedText(f.CurrentConditions.Conditions) || !boundedText(f.CurrentConditions.Icon) {
		return nil, fmt.Errorf("%w: forecast text exceeds %d bytes", ErrMalformedResponse, maxTextBytes)
	}
	if err := validateForecastCurrent(f.CurrentConditions); err != nil {
		return nil, fmt.Errorf("%w: current forecast: %w", ErrMalformedResponse, err)
	}
	for index, day := range f.Forecast.Daily {
		if !boundedText(day.Conditions) || !boundedText(day.Icon) || !boundedText(day.PrecipType) {
			return nil, fmt.Errorf("%w: daily forecast text exceeds %d bytes", ErrMalformedResponse, maxTextBytes)
		}
		if err := validateDailyForecast(day); err != nil {
			return nil, fmt.Errorf("%w: daily forecast %d: %w", ErrMalformedResponse, index, err)
		}
	}
	for index, hour := range f.Forecast.Hourly {
		if !boundedText(hour.Conditions) || !boundedText(hour.Icon) {
			return nil, fmt.Errorf("%w: hourly forecast text exceeds %d bytes", ErrMalformedResponse, maxTextBytes)
		}
		if err := validateHourlyForecast(hour); err != nil {
			return nil, fmt.Errorf("%w: hourly forecast %d: %w", ErrMalformedResponse, index, err)
		}
	}
	return &f, nil
}

func validateForecastCurrent(current ForecastCurrent) error {
	if current.Time <= 0 || current.Time > model.MaxEpochSeconds ||
		!optionalFloatInRange(current.AirTemperature, -150, 150) ||
		!optionalFloatInRange(current.FeelsLike, -200, 200) ||
		!optionalFloatInRange(current.RelativeHumidity, 0, 100) {
		return errors.New("fields are outside accepted bounds")
	}
	return nil
}

func validateDailyForecast(day DailyForecast) error {
	if day.DayStartLocal <= 0 || day.DayStartLocal > model.MaxEpochSeconds ||
		!optionalEpoch(day.Sunrise) || !optionalEpoch(day.Sunset) ||
		!optionalFloatInRange(day.AirTempHigh, -150, 150) ||
		!optionalFloatInRange(day.AirTempLow, -150, 150) ||
		!optionalProbability(day.PrecipProbability) {
		return errors.New("fields are outside accepted bounds")
	}
	return nil
}

func validateHourlyForecast(hour HourlyForecast) error {
	if hour.Time <= 0 || hour.Time > model.MaxEpochSeconds ||
		!optionalFloatInRange(hour.AirTemperature, -150, 150) ||
		!optionalFloatInRange(hour.FeelsLike, -200, 200) ||
		!optionalFloatInRange(hour.RelativeHumidity, 0, 100) ||
		!optionalProbability(hour.PrecipProbability) ||
		!optionalFloatInRange(hour.WindAvg, 0, 200) ||
		!optionalFloatInRange(hour.WindGust, 0, 200) ||
		!optionalFloatInRange(hour.WindDirection, 0, 360) ||
		!optionalFloatInRange(hour.UV, 0, 100) {
		return errors.New("fields are outside accepted bounds")
	}
	return nil
}

func optionalEpoch(epoch int64) bool {
	return epoch == 0 || epoch > 0 && epoch <= model.MaxEpochSeconds
}

func optionalFloatInRange(value *float64, minimum, maximum float64) bool {
	return value == nil || finiteInRange(*value, minimum, maximum)
}

func optionalProbability(value *int) bool {
	return value == nil || *value >= 0 && *value <= 100
}

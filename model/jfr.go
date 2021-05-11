package model

import (
	"encoding/binary"
	"fmt"
	"github.com/elastic/apm-server/datastreams"
	"github.com/elastic/apm-server/transform"
	"github.com/elastic/beats/v7/libbeat/beat"
	"github.com/elastic/beats/v7/libbeat/common"
	"golang.org/x/text/encoding/charmap"
	"io"
	"io/ioutil"
	"sort"
	"strconv"
	"time"
)

const (
	metaOffset      = 24
	chunkHeaderSize = 68
	cpoolOffset     = 16
)

type JfrProfileEvent struct {
	Metadata Metadata
	Profile  *JfrProfile
}

type FlameGraph struct {
	samples int64
	children map[string]*FlameGraph
}

func (f *FlameGraph) addChild(signature string, samples int64) *FlameGraph {
	frame, ok := f.children[signature]
	if !ok {
		frame = &FlameGraph{
			samples:  0,
			children: map[string]*FlameGraph{},
		}
		f.children[signature] = frame
	}
	frame.samples += samples
	return frame
}

type FlatFlameGraph struct {
	levels  []int
	samples []int64
	typ     []byte
	titles  []int
	symbols Symbols
}

func NewFlatFlameGraph(f *FlameGraph) *FlatFlameGraph {
	flattened := &FlatFlameGraph{
		levels:  make([]int, 0, 1000),
		samples: make([]int64, 0, 1000),
		typ:     make([]byte, 0, 1000),
		titles:  make([]int, 0, 1000),
		symbols: Symbols{
			symbolToId: make(map[string]int, 1000),
			symbols:    make([]string, 0, 1000),
		},
	}
	for sig, child := range f.children {
		flattened.addFrame(0, sig, child)
	}
	return flattened
}

func (f *FlatFlameGraph) addFrame(level int, signature string, frame *FlameGraph) {
	f.levels = append(f.levels, level)
	f.samples = append(f.samples, frame.samples)
	f.titles = append(f.titles, f.symbols.symbolize(signature))
	for sig, child := range frame.children {
		f.addFrame(level + 1, sig, child)
	}
}

type Symbols struct {
	symbolToId map[string]int
	symbols    []string
}

func (s *Symbols) symbolize(str string) int {
	if symbolId, ok := s.symbolToId[str]; ok {
		return symbolId
	}
	symbolId := len(s.symbolToId)
	s.symbolToId[str] = symbolId
	s.symbols = append(s.symbols, str)
	return symbolId
}

func (s *Symbols) resolveSymbol(symbolId int) string {
	return s.symbols[symbolId]
}

func NewFlameGraph(jp *JfrProfileEvent) *FlameGraph {
	root := &FlameGraph{
		samples:  0,
		children: map[string]*FlameGraph{},
	}
	for _, stackTrace := range jp.Profile.stackTraces {
		current := root
		for _, method := range stackTrace.methods {
			methodRef := jp.Profile.methods[method]

			var signature string
			methodName := jp.Profile.symbols[methodRef.name]
			if className := jp.Profile.symbols[methodRef.cls]; len(className) > 0 {
				signature = className + "." + methodName
			} else {
				signature = methodName
			}
			current = current.addChild(signature, stackTrace.samples)
		}
	}
	return root
}
func (jp JfrProfileEvent) appendBeatEvents(cfg *transform.Config, events []beat.Event) []beat.Event {
	profileTimestamp := time.Unix(0, jp.Profile.startNanos)
	flatTree := NewFlatFlameGraph(NewFlameGraph(&jp))
	profileFields := common.MapStr{}
	profileFields["levels"] = flatTree.levels
	profileFields["samples"] = flatTree.samples
	profileFields["titles"] = flatTree.titles
	profileFields["symbols"] = flatTree.symbols.symbols

	fields := mapStr{
		"processor":    profileProcessorEntry,
		profileDocType: profileFields,
	}
	if cfg.DataStreams {
		fields[datastreams.TypeField] = datastreams.MetricsType
		dataset := fmt.Sprintf("%s.%s", ProfilesDataset, datastreams.NormalizeServiceName(jp.Metadata.Service.Name))
		fields[datastreams.DatasetField] = dataset
	}
	events = append(events, beat.Event{
		Timestamp: profileTimestamp,
		Fields:    common.MapStr(fields),
	})
	return events
}

type JfrProfile struct {
	buf []byte
	pos int

	startTicks    int64
	startNanos    int64
	durationNanos int64
	ticksPerSec   int64
	types         map[int]*JfrClass
	typesByName   map[string]*JfrClass
	threads       map[int64]string
	classes       map[int64]int64
	symbols       map[int64]string
	methods       map[int64]*MethodRef
	stackTraces   map[int64]*StackTrace
	frameTypes    map[int]string
	threadStates  map[int]string
	samples       []*JfrSample
}

type Element interface {
	addChild(e *Element)
}

type JfrClass struct {
	id     int
	name   string
	fields []*JfrField
}

func (c *JfrClass) addChild(e *Element) {
	if field, ok := (*e).(*JfrField); ok {
		c.fields = append(c.fields, field)
	}
}

func NewJfrClass(a map[string]string) *JfrClass {
	id, _ := strconv.Atoi(a["id"])
	return &JfrClass{
		id:     id,
		name:   a["name"],
		fields: make([]*JfrField, 0),
	}
}

func (c *JfrClass) field(name string) *JfrField {
	for _, field := range c.fields {
		if field.name == name {
			return field
		}
	}
	return nil
}

type JfrField struct {
	name         string
	typ          int
	constantPool bool
}

func NewJfrField(a map[string]string) *JfrField {
	typ, _ := strconv.Atoi(a["class"])
	return &JfrField{
		name:         a["name"],
		typ:          typ,
		constantPool: a["constantPool"] == "true",
	}
}

func (f *JfrField) addChild(e *Element) {
	// noop
}

type NoopElement struct {
}

func (n *NoopElement) addChild(e *Element) {
	// noop
}

type MethodRef struct {
	cls  int64
	name int64
	sig  int64
}
type StackTrace struct {
	methods []int64
	types   []int8
	samples int64
}
type JfrSample struct {
	time         int64
	tid          int
	stackTraceId int
	threadState  int
}

func ParseJfrProfile(r io.Reader) (*JfrProfile, error) {

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	j := JfrProfile{
		buf:           b,
		pos:           0,
		startNanos:    0,
		durationNanos: 0,
		startTicks:    0,
		ticksPerSec:   0,
		types:         make(map[int]*JfrClass),
		typesByName:   make(map[string]*JfrClass),
		frameTypes:    make(map[int]string),
		threadStates:  make(map[int]string),
		samples:       make([]*JfrSample, 0),
	}

	magicByte := j.getInt32()
	if magicByte != 0x464c5200 {
		return nil, fmt.Errorf("not a valid JFR file")
	}
	version := j.getInt32()
	if version < 0x20000 || version > 0x2ffff {
		return nil, fmt.Errorf("unsupported JFR version: %d.%d", version>>16, version&0xffff)
	}
	j.position(32)
	j.startNanos = j.getInt64()
	j.durationNanos = j.getInt64()
	j.startTicks = j.getInt64()
	j.ticksPerSec = j.getInt64()
	j.readMeta()
	j.readConstantPool()
	j.readEvents()
	return &j, nil
}

func (j *JfrProfile) readMeta() {
	off := j.getInt32At(metaOffset + 4)
	j.position(off)
	// we need to consume the var ints to progress the position appropriately but we don't actually use the values
	j.getVarint32()
	j.getVarint32()
	j.getVarint64()
	j.getVarint64()
	j.getVarint64()

	strings := make([]string, j.getVarint32())
	for i := range strings {
		strings[i], _, _ = j.getString()
	}
	j.readElement(strings)
}

func (j *JfrProfile) readElement(strings []string) *Element {
	name := strings[j.getVarint32()]
	attributeCount := j.getVarint32()
	attributes := make(map[string]string, attributeCount)
	for i := 0; i < attributeCount; i++ {
		attributes[strings[j.getVarint32()]] = strings[j.getVarint32()]
	}
	e := j.createElement(name, attributes)
	childCount := j.getVarint32()
	for i := 0; i < childCount; i++ {
		e.addChild(j.readElement(strings))
	}
	return &e
}

func (j *JfrProfile) createElement(name string, attributes map[string]string) Element {
	switch name {
	case "class":
		typ := NewJfrClass(attributes)
		if _, ok := attributes["superType"]; !ok {
			j.types[typ.id] = typ
		}
		j.typesByName[typ.name] = typ
		return typ
	case "field":
		return NewJfrField(attributes)
	default:
		return &NoopElement{}
	}
}

func (j *JfrProfile) readConstantPool() {
	off := j.getInt32At(cpoolOffset + 4)
	for true {
		j.position(off)
		j.getVarint32()
		j.getVarint32()
		j.getVarint64()
		j.getVarint64()
		delta := int(j.getVarint64())
		j.getVarint32()
		poolCount := j.getVarint32()
		for i := 0; i < poolCount; i++ {
			typ := j.getVarint32()
			j.readConstants(j.types[typ])
		}
		if delta == 0 {
			break
		}
		off += delta
	}
}

func (j *JfrProfile) readConstants(typ *JfrClass) {
	switch typ.name {
	case "jdk.types.ChunkHeader":
		// skip
		j.position(j.pos + (chunkHeaderSize + 3))
		break
	case "java.lang.Thread":
		j.readThreads(typ.field("group") != nil)
		break
	case "java.lang.Class":
		j.readClasses(typ.field("hidden") != nil)
		break
	case "jdk.types.Symbol":
		_ = j.readSymbols()
		break
	case "jdk.types.Method":
		j.readMethods()
		break
	case "jdk.types.StackTrace":
		j.readStackTraces()
		break
	case "jdk.types.FrameType":
		j.readMap(j.frameTypes)
		break
	case "jdk.types.ThreadState":
		j.readMap(j.threadStates)
		break
	default:
		j.readOtherConstants(typ.fields)
	}

}
func (j *JfrProfile) readThreads(hasGroup bool) {
	count := j.getVarint32()
	j.threads = make(map[int64]string, count)
	for i := 0; i < count; i++ {
		id := j.getVarint64()
		osName, _, _ := j.getString()
		// os thread id
		j.getVarint32()
		javaName, hasJavaName, _ := j.getString()
		// java thread id
		j.getVarint64()
		if hasGroup {
			j.getVarint64()
		}
		if hasJavaName {
			j.threads[id] = javaName
		} else {
			j.threads[id] = osName
		}
	}
}

func (j *JfrProfile) readClasses(hasHidden bool) {
	count := j.getVarint32()
	j.classes = make(map[int64]int64, count)
	for i := 0; i < count; i++ {
		id := j.getVarint64()
		j.getVarint64()
		name := j.getVarint64()
		j.getVarint64()
		j.getVarint32()
		if hasHidden {
			j.getVarint32()
		}
		j.classes[id] = name
	}
}

func (j *JfrProfile) readMethods() {
	count := j.getVarint32()
	j.methods = make(map[int64]*MethodRef, count)
	for i := 0; i < count; i++ {
		id := j.getVarint64()
		cls := j.getVarint64()
		name := j.getVarint64()
		sig := j.getVarint64()
		j.getVarint32() // modifiers
		j.getVarint32() // hidden

		j.methods[id] = &MethodRef{cls, name, sig}
	}
}

func (j *JfrProfile) readSymbols() error {
	count := j.getVarint32()
	j.symbols = make(map[int64]string, count)
	for i := 0; i < count; i++ {
		id := j.getVarint64()
		if j.getByte() != 3 {
			return fmt.Errorf("invalid symbol encoding")
		}
		bytes := j.getBytes()
		j.symbols[id] = string(bytes)
	}
	return nil
}

func (j *JfrProfile) readStackTraces() {
	count := j.getVarint32()
	j.stackTraces = make(map[int64]*StackTrace, count)
	for i := 0; i < count; i++ {
		id := j.getVarint64()
		j.getVarint32() // truncated
		j.stackTraces[id] = j.readStackTrace()
	}
}

func (j *JfrProfile) readStackTrace() *StackTrace {
	depth := j.getVarint32()
	methods := make([]int64, depth)
	types := make([]int8, depth)
	for i := 0; i < depth; i++ {
		methods[i] = j.getVarint64()
		j.getVarint32() // line
		j.getVarint32() // bci
		types[i] = j.getByte()
	}
	return &StackTrace{
		methods: methods,
		types:   types,
	}
}

func (j *JfrProfile) readMap(m map[int]string) {
	count := j.getVarint32()
	for i := 0; i < count; i++ {
		m[j.getVarint32()], _, _ = j.getString()
	}
}

func (j *JfrProfile) readOtherConstants(fields []*JfrField) {
	stringType := j.getTypeId("java.lang.String")
	numeric := make([]bool, len(fields))
	for i, f := range fields {
		numeric[i] = f.constantPool || f.typ != stringType
	}
	count := j.getVarint32()
	for i := 0; i < count; i++ {
		j.getVarint64()
		j.readFields(numeric)
	}
}

func (j *JfrProfile) readFields(numeric []bool) {
	for _, n := range numeric {
		if n {
			j.getVarint64()
		} else {
			j.getString()
		}
	}
}

func (j *JfrProfile) getTypeId(name string) int {
	class := j.typesByName[name]
	if class != nil {
		return class.id
	} else {
		return -1
	}
}

func (j *JfrProfile) readEvents() {
	executionSample := j.getTypeId("jdk.ExecutionSample")
	nativeMethodSample := j.getTypeId("jdk.NativeMethodSample")
	j.position(chunkHeaderSize)
	for j.hasRemaining() {
		pos := j.pos
		size := j.getVarint32()
		typ := j.getVarint32()
		if typ == executionSample || typ == nativeMethodSample {
			j.readExecutionSample()
		} else {
			j.position(pos + size)
		}
	}
	samples := j.samples
	sort.Slice(samples, func(i, j int) bool {
		s1 := samples[i]
		s2 := samples[j]
		//if s1.tid != s2.tid {
		//	return s1.tid < s2.tid
		//} else {
			return s1.time < s2.time
		//}
	})
}

func (j *JfrProfile) readExecutionSample() {
	time := j.getVarint64()
	tid := j.getVarint32()
	stackTraceId := j.getVarint32()
	threadState := j.getVarint32()
	j.samples = append(j.samples, &JfrSample{time, tid, stackTraceId, threadState})
	if s, ok := j.stackTraces[int64(stackTraceId)]; ok {
		s.samples++
	}
}

func (j *JfrProfile) position(off int) {
	j.pos = off
}

func (j *JfrProfile) hasRemaining() bool {
	return j.pos < len(j.buf)
}

func (j *JfrProfile) getByte() int8 {
	b := j.buf[j.pos]
	j.pos++
	return int8(b)
}

func (j *JfrProfile) getBytes() []byte {
	size := j.getVarint32()
	b := j.buf[j.pos : j.pos+size]
	j.pos += size
	return b
}

func (j *JfrProfile) getInt32() int32 {
	i := int32(binary.BigEndian.Uint32(j.buf[j.pos:]))
	j.pos += 4
	return i
}

func (j *JfrProfile) getVarint32() int {
	result := 0
	for shift := 0; ; shift += 7 {
		getByte := j.getByte()
		b := int(getByte)
		result |= (b & 0x7f) << shift
		if b >= 0 {
			return result
		}
	}
}

func (j *JfrProfile) getVarint64() int64 {
	result := int64(0)
	for shift := 0; shift < 56; shift += 7 {
		b := int64(j.getByte())
		result |= (b & 0x7f) << shift
		if b >= 0 {
			return result
		}
	}
	return result | (int64(j.getByte())&int64(0xff))<<int64(56)
}

func (j *JfrProfile) getInt64() int64 {
	i := int64(binary.BigEndian.Uint64(j.buf[j.pos:]))
	j.pos += 8
	return i
}

func (j *JfrProfile) getInt32At(off int) int {
	return int(binary.BigEndian.Uint32(j.buf[off:]))
}

func (j *JfrProfile) getStringPointer() *string {
	if s, ok, _ := j.getString(); ok {
		return &s
	}
	return nil
}
func (j *JfrProfile) getString() (string, bool, error) {
	encoding := j.getByte()
	switch encoding {
	case 0:
		return "", false, nil
	case 1:
		return "", true, nil
	case 3:
		return string(j.getBytes()), true, nil
	case 4:
		length := j.getVarint32()
		runes := make([]rune, length)
		for i := 0; i < length; i++ {
			runes[i] = rune(j.getVarint32())
		}
		return string(runes), true, nil
	case 5:
		if bytes, err := charmap.ISO8859_1.NewDecoder().Bytes(j.getBytes()); err == nil {
			return string(bytes), true, nil
		} else {
			return "", false, err
		}
	default:
		return "", true, fmt.Errorf("invalid string encoding %d", encoding)
	}
}

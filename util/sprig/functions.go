package sprig

import (
	"errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"math/rand"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// TxtFuncMap produces the function map.
//
// Use this to pass the functions into the template engine:
//
//	tpl := template.New("foo").Funcs(sprig.FuncMap()))
//
// TxtFuncMap returns a 'text/template'.FuncMap
func TxtFuncMap() template.FuncMap {
	gfm := make(map[string]any, len(genericMap))
	for k, v := range genericMap {
		gfm[k] = v
	}
	return gfm
}

var genericMap = map[string]any{
	// Date functions
	"ago":              dateAgo,
	"date":             date,
	"date_in_zone":     dateInZone,
	"date_modify":      dateModify,
	"dateInZone":       dateInZone,
	"dateModify":       dateModify,
	"duration":         duration,
	"durationRound":    durationRound,
	"htmlDate":         htmlDate,
	"htmlDateInZone":   htmlDateInZone,
	"must_date_modify": mustDateModify,
	"mustDateModify":   mustDateModify,
	"mustToDate":       mustToDate,
	"now":              time.Now,
	"toDate":           toDate,
	"unixEpoch":        unixEpoch,

	// Strings
	"trunc": trunc,
	"trim":  strings.TrimSpace,
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"title": func(s string) string {
		return cases.Title(language.English).String(s)
	},
	"substr": substring,
	// Switch order so that "foo" | repeat 5
	"repeat": func(count int, str string) string { return strings.Repeat(str, count) },
	// Deprecated: Use trimAll.
	"trimall": func(a, b string) string { return strings.Trim(b, a) },
	// Switch order so that "$foo" | trimall "$"
	"trimAll":    func(a, b string) string { return strings.Trim(b, a) },
	"trimSuffix": func(a, b string) string { return strings.TrimSuffix(b, a) },
	"trimPrefix": func(a, b string) string { return strings.TrimPrefix(b, a) },
	// Switch order so that "foobar" | contains "foo"
	"contains":   func(substr string, str string) bool { return strings.Contains(str, substr) },
	"hasPrefix":  func(substr string, str string) bool { return strings.HasPrefix(str, substr) },
	"hasSuffix":  func(substr string, str string) bool { return strings.HasSuffix(str, substr) },
	"quote":      quote,
	"squote":     squote,
	"cat":        cat,
	"indent":     indent,
	"nindent":    nindent,
	"replace":    replace,
	"plural":     plural,
	"sha1sum":    sha1sum,
	"sha256sum":  sha256sum,
	"sha512sum":  sha512sum,
	"adler32sum": adler32sum,
	"toString":   strval,

	// Wrap Atoi to stop errors.
	"atoi":      func(a string) int { i, _ := strconv.Atoi(a); return i },
	"seq":       seq,
	"toDecimal": toDecimal,

	// split "/" foo/bar returns map[int]string{0: foo, 1: bar}
	"split":     split,
	"splitList": func(sep, orig string) []string { return strings.Split(orig, sep) },
	// splitn "/" foo/bar/fuu returns map[int]string{0: foo, 1: bar/fuu}
	"splitn":    splitn,
	"toStrings": strslice,

	"until":     until,
	"untilStep": untilStep,

	// VERY basic arithmetic.
	"add1": func(i any) int64 { return toInt64(i) + 1 },
	"add": func(i ...any) int64 {
		var a int64 = 0
		for _, b := range i {
			a += toInt64(b)
		}
		return a
	},
	"sub": func(a, b any) int64 { return toInt64(a) - toInt64(b) },
	"div": func(a, b any) int64 { return toInt64(a) / toInt64(b) },
	"mod": func(a, b any) int64 { return toInt64(a) % toInt64(b) },
	"mul": func(a any, v ...any) int64 {
		val := toInt64(a)
		for _, b := range v {
			val = val * toInt64(b)
		}
		return val
	},
	"randInt": func(min, max int) int { return rand.Intn(max-min) + min },
	"biggest": max,
	"max":     max,
	"min":     min,
	"maxf":    maxf,
	"minf":    minf,
	"ceil":    ceil,
	"floor":   floor,
	"round":   round,

	// string slices. Note that we reverse the order b/c that's better
	// for template processing.
	"join":      join,
	"sortAlpha": sortAlpha,

	// Defaults
	"default":          dfault,
	"empty":            empty,
	"coalesce":         coalesce,
	"all":              all,
	"any":              anyNonEmpty,
	"compact":          compact,
	"mustCompact":      mustCompact,
	"fromJSON":         fromJSON,
	"toJSON":           toJSON,
	"toPrettyJSON":     toPrettyJSON,
	"toRawJSON":        toRawJSON,
	"mustFromJSON":     mustFromJSON,
	"mustToJSON":       mustToJSON,
	"mustToPrettyJSON": mustToPrettyJSON,
	"mustToRawJSON":    mustToRawJSON,
	"ternary":          ternary,

	// Reflection
	"typeOf":     typeOf,
	"typeIs":     typeIs,
	"typeIsLike": typeIsLike,
	"kindOf":     kindOf,
	"kindIs":     kindIs,
	"deepEqual":  reflect.DeepEqual,

	// Paths:
	"base":  path.Base,
	"dir":   path.Dir,
	"clean": path.Clean,
	"ext":   path.Ext,
	"isAbs": path.IsAbs,

	// Filepaths:
	"osBase":  filepath.Base,
	"osClean": filepath.Clean,
	"osDir":   filepath.Dir,
	"osExt":   filepath.Ext,
	"osIsAbs": filepath.IsAbs,

	// Encoding:
	"b64enc": base64encode,
	"b64dec": base64decode,
	"b32enc": base32encode,
	"b32dec": base32decode,

	// Data Structures:
	"tuple":  list, // FIXME: with the addition of append/prepend these are no longer immutable.
	"list":   list,
	"dict":   dict,
	"get":    get,
	"set":    set,
	"unset":  unset,
	"hasKey": hasKey,
	"pluck":  pluck,
	"keys":   keys,
	"pick":   pick,
	"omit":   omit,
	"values": values,

	"append":      push,
	"push":        push,
	"mustAppend":  mustPush,
	"mustPush":    mustPush,
	"prepend":     prepend,
	"mustPrepend": mustPrepend,
	"first":       first,
	"mustFirst":   mustFirst,
	"rest":        rest,
	"mustRest":    mustRest,
	"last":        last,
	"mustLast":    mustLast,
	"initial":     initial,
	"mustInitial": mustInitial,
	"reverse":     reverse,
	"mustReverse": mustReverse,
	"uniq":        uniq,
	"mustUniq":    mustUniq,
	"without":     without,
	"mustWithout": mustWithout,
	"has":         has,
	"mustHas":     mustHas,
	"slice":       slice,
	"mustSlice":   mustSlice,
	"concat":      concat,
	"dig":         dig,
	"chunk":       chunk,
	"mustChunk":   mustChunk,

	// UUIDs:
	"uuidv4": uuidv4,

	// Flow Control:
	"fail": func(msg string) (string, error) { return "", errors.New(msg) },

	// Regex
	"regexMatch":                 regexMatch,
	"mustRegexMatch":             mustRegexMatch,
	"regexFindAll":               regexFindAll,
	"mustRegexFindAll":           mustRegexFindAll,
	"regexFind":                  regexFind,
	"mustRegexFind":              mustRegexFind,
	"regexReplaceAll":            regexReplaceAll,
	"mustRegexReplaceAll":        mustRegexReplaceAll,
	"regexReplaceAllLiteral":     regexReplaceAllLiteral,
	"mustRegexReplaceAllLiteral": mustRegexReplaceAllLiteral,
	"regexSplit":                 regexSplit,
	"mustRegexSplit":             mustRegexSplit,
	"regexQuoteMeta":             regexQuoteMeta,

	// URLs:
	"urlParse": urlParse,
	"urlJoin":  urlJoin,
}

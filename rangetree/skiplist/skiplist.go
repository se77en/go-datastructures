package skiplist

import (
	"log"

	"github.com/Workiva/go-datastructures/rangetree"
	"github.com/Workiva/go-datastructures/slice/skip"
)

func init() {
	log.Printf(`I HATE THIS`)
}

func isLastDimension(dimension, lastDimension uint64) bool {
	if dimension >= lastDimension { // useful in testing and denotes a serious problem
		panic(`Dimension is greater than possible dimensions.`)
	}

	return dimension == lastDimension-1
}

func needsDeletion(value, index, number int64) bool {
	if number > 0 {
		return false
	}

	number = -number // get the magnitude
	offset := value - index

	return offset >= 0 && offset < number
}

type dimensionalBundle struct {
	key uint64
	sl  *skip.SkipList
}

// Key returns the key for this bundle.  Fulfills skip.Entry interface.
func (db *dimensionalBundle) Key() uint64 {
	return db.key
}

type lastBundle struct {
	key   uint64
	entry rangetree.Entry
}

// Key returns the key for this bundle.  Fulfills skip.Entry interface.
func (lb *lastBundle) Key() uint64 {
	return lb.key
}

type skipListRT struct {
	top                *skip.SkipList
	dimensions, number uint64
}

func (rt *skipListRT) init(dimensions uint64) {
	rt.dimensions = dimensions
	rt.top = skip.New(uint64(0))
}

func (rt *skipListRT) add(entry rangetree.Entry) rangetree.Entry {
	var (
		value int64
		e     skip.Entry
		sl    = rt.top
		db    *dimensionalBundle
		lb    *lastBundle
	)

	for i := uint64(0); i < rt.dimensions; i++ {
		value = entry.ValueAtDimension(i)
		e = sl.Get(uint64(value))[0]
		if isLastDimension(i, rt.dimensions) {
			if e != nil { // this is an overwrite
				lb = e.(*lastBundle)
				oldEntry := lb.entry
				lb.entry = entry
				return oldEntry
			}

			// need to add new sl entry
			lb = &lastBundle{key: uint64(value), entry: entry}
			rt.number++
			sl.Insert(lb)
			return nil
		}

		if e == nil { // we need the intermediate dimension
			db = &dimensionalBundle{key: uint64(value), sl: skip.New(uint64(0))}
			sl.Insert(db)
		} else {
			db = e.(*dimensionalBundle)
		}

		sl = db.sl
	}

	panic(`Ran out of dimensions before for loop completed.`)
}

func (rt *skipListRT) Add(entries ...rangetree.Entry) rangetree.Entries {
	overwritten := make(rangetree.Entries, 0, len(entries))
	for _, e := range entries {
		overwritten = append(overwritten, rt.add(e))
	}

	return overwritten
}

func (rt *skipListRT) get(entry rangetree.Entry) rangetree.Entry {
	var (
		sl    = rt.top
		e     skip.Entry
		value uint64
	)
	for i := uint64(0); i < rt.dimensions; i++ {
		value = uint64(entry.ValueAtDimension(i))
		e = sl.Get(value)[0]
		if e == nil {
			return nil
		}

		if isLastDimension(i, rt.dimensions) {
			return e.(*lastBundle).entry
		}

		sl = e.(*dimensionalBundle).sl
	}

	panic(`Reached past for loop without finding last dimension.`)
}

func (rt *skipListRT) Get(entries ...rangetree.Entry) rangetree.Entries {
	results := make(rangetree.Entries, 0, len(entries))
	for _, e := range entries {
		results = append(results, rt.get(e))
	}

	return results
}

func (rt *skipListRT) Len() uint64 {
	return rt.number
}

func (rt *skipListRT) deleteRecursive(sl *skip.SkipList, dimension uint64,
	entry rangetree.Entry) rangetree.Entry {

	value := entry.ValueAtDimension(dimension)
	if isLastDimension(dimension, rt.dimensions) {
		entries := sl.Delete(uint64(value))
		if entries[0] == nil {
			return nil
		}

		rt.number--
		return entries[0].(*lastBundle).entry
	}

	db, ok := sl.Get(uint64(value))[0].(*dimensionalBundle)
	if !ok { // value was not found
		return nil
	}

	result := rt.deleteRecursive(db.sl, dimension+1, entry)
	if result == nil {
		return nil
	}

	if db.sl.Len() == 0 {
		sl.Delete(db.key)
	}

	return result
}

func (rt *skipListRT) delete(entry rangetree.Entry) rangetree.Entry {
	return rt.deleteRecursive(rt.top, 0, entry)
}

func (rt *skipListRT) Delete(entries ...rangetree.Entry) {
	for _, e := range entries {
		rt.delete(e)
	}
}

func (rt *skipListRT) apply(sl *skip.SkipList, dimension uint64,
	interval rangetree.Interval, fn func(rangetree.Entry) bool) bool {

	lowValue, highValue := interval.LowAtDimension(dimension), interval.HighAtDimension(dimension)

	var e skip.Entry

	for iter := sl.Iter(uint64(lowValue)); iter.Next(); {
		e = iter.Value()
		if int64(e.Key()) >= highValue {
			break
		}

		if isLastDimension(dimension, rt.dimensions) {
			if !fn(e.(*lastBundle).entry) {
				return false
			}
		} else {

			if !rt.apply(e.(*dimensionalBundle).sl, dimension+1, interval, fn) {
				return false
			}
		}
	}

	return true
}

func (rt *skipListRT) Apply(interval rangetree.Interval, fn func(rangetree.Entry) bool) {
	rt.apply(rt.top, 0, interval, fn)
}

func (rt *skipListRT) Query(interval rangetree.Interval) rangetree.Entries {
	entries := make(rangetree.Entries, 0, 100)
	rt.apply(rt.top, 0, interval, func(e rangetree.Entry) bool {
		entries = append(entries, e)
		return true
	})

	return entries
}

func (rt *skipListRT) flatten(sl *skip.SkipList, dimension uint64, entries *rangetree.Entries) {
	lastDimension := isLastDimension(dimension, rt.dimensions)
	for iter := sl.Iter(0); iter.Next(); {
		if lastDimension {
			*entries = append(*entries, iter.Value().(*lastBundle).entry)
		} else {
			rt.flatten(iter.Value().(*dimensionalBundle).sl, dimension+1, entries)
		}
	}
}

func (rt *skipListRT) insert(sl *skip.SkipList, dimension, insertDimension uint64,
	index, number int64, deleted, affected *rangetree.Entries) {

	var e skip.Entry
	lastDimension := isLastDimension(dimension, rt.dimensions)
	affectedDimension := dimension == insertDimension
	var iter skip.Iterator
	if dimension == insertDimension {
		iter = sl.Iter(uint64(index))
	} else {
		iter = sl.Iter(0)
	}

	var toDelete skip.Entries
	if number < 0 {
		toDelete = make(skip.Entries, 0, 100)
	}

	for iter.Next() {
		e = iter.Value()
		if !affectedDimension {
			rt.insert(e.(*dimensionalBundle).sl, dimension+1,
				insertDimension, index, number, deleted, affected,
			)
			continue
		}
		if needsDeletion(int64(e.Key()), index, number) {
			toDelete = append(toDelete, e)
			continue
		}

		if lastDimension {
			e.(*lastBundle).key += uint64(number)
			*affected = append(*affected, e.(*lastBundle).entry)
		} else {
			e.(*dimensionalBundle).key += uint64(number)
			rt.flatten(e.(*dimensionalBundle).sl, dimension+1, affected)
		}
	}

	if len(toDelete) > 0 {
		keys := make([]uint64, 0, len(toDelete))
		for _, e := range toDelete {
			if lastDimension {
				*deleted = append(*deleted, e.(*lastBundle).entry)
			} else {
				rt.flatten(e.(*dimensionalBundle).sl, dimension+1, deleted)
			}
			keys = append(keys, e.Key())
		}

		sl.Delete(keys...)
	}
}

func (rt *skipListRT) InsertAtDimension(dimension uint64,
	index, number int64) (rangetree.Entries, rangetree.Entries) {

	if dimension >= rt.dimensions || number == 0 {
		return rangetree.Entries{}, rangetree.Entries{}
	}

	affected := make(rangetree.Entries, 0, 100)
	var deleted rangetree.Entries
	if number < 0 {
		deleted = make(rangetree.Entries, 0, 100)
	}

	rt.insert(rt.top, 0, dimension, index, number, &deleted, &affected)
	rt.number -= uint64(len(deleted))
	return affected, deleted
}

func new(dimensions uint64) *skipListRT {
	sl := &skipListRT{}
	sl.init(dimensions)
	return sl
}

func New(dimensions uint64) rangetree.RangeTree {
	return new(dimensions)
}
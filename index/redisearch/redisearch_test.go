package redisearch

import (
	"fmt"
	"testing"

	"github.com/RedisLabs/RediSearchBenchmark/index"
	"github.com/RedisLabs/RediSearchBenchmark/query"
	"github.com/stretchr/testify/assert"
)

func TestIndex(t *testing.T) {
	// todo: run redisearch automatically
	t.SkipNow()
	md := index.NewMetadata().AddField(index.NewTextField("title", 1.0)).
		AddField(index.NewNumericField("score"))

	idx := NewIndex("localhost:6379", "testung", md)

	docs := []index.Document{
		index.NewDocument("doc1", 0.1).Set("title", "hello world").Set("score", 1),
		index.NewDocument("doc2", 1.0).Set("title", "foo bar hello").Set("score", 2),
	}

	assert.NoError(t, idx.Drop())
	assert.NoError(t, idx.Create())

	assert.NoError(t, idx.Index(docs, nil))

	q := query.NewQuery(idx.name, "hello world")
	docs, total, err := idx.Search(*q)
	assert.NoError(t, err)
	assert.True(t, total > 0)
	assert.Len(t, docs, 1)
	assert.Equal(t, docs[0].Id, "doc1")
	assert.Equal(t, docs[0].Properties["title"], "hello world")

	q = query.NewQuery(idx.name, "hello")
	docs, total, err = idx.Search(*q)
	assert.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, docs, 2)
	assert.Equal(t, docs[0].Id, "doc2")
	assert.Equal(t, docs[1].Id, "doc1")

}

func TestDistributedIndex(t *testing.T) {
	// todo: run redisearch automatically
	//t.SkipNow()
	md := index.NewMetadata().AddField(index.NewTextField("title", 1.0)).
		AddField(index.NewNumericField("score"))

	idx := NewDistributedIndex("dtest", []string{"localhost:6379"}, 2, md)

	docs := []index.Document{
		index.NewDocument("doc1", 0.1).Set("title", "hello world").Set("score", 1),
		index.NewDocument("doc2", 1.0).Set("title", "foo bar hello").Set("score", 2),
	}

	assert.NoError(t, idx.Drop())
	assert.NoError(t, idx.Create())

	assert.NoError(t, idx.Index(docs, nil))

	q := query.NewQuery("dtest", "hello world")
	docs, total, err := idx.Search(*q)
	assert.NoError(t, err)
	assert.True(t, total > 0)
	assert.Len(t, docs, 1)
	assert.Equal(t, docs[0].Id, "doc1")
	assert.Equal(t, docs[0].Properties["title"], "hello world")

	q = query.NewQuery("dtest", "hello")
	docs, total, err = idx.Search(*q)
	t.Log(docs, total, err)
	assert.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, docs, 2)
	assert.Equal(t, docs[0].Id, "doc2")
	assert.Equal(t, docs[1].Id, "doc1")

	suggs := []index.Suggestion{}
	for i := 0; i < 100; i++ {
		suggs = append(suggs, index.Suggestion{fmt.Sprintf("suggestion %d", i), float64(i)})
	}

	assert.NoError(t, idx.AddTerms(suggs...))

	suggs, err = idx.Suggest("sugg", 10, false)
	assert.NoError(t, err)
	fmt.Println(suggs)
	assert.Len(t, suggs, 10)

}

func TestAutocompleter(t *testing.T) {

	ac := NewAutocompleter("localhost:6379", "ac")

	assert.NotNil(t, ac)
	assert.NoError(t, ac.AddTerms(
		index.Suggestion{"hello world", 1},
		index.Suggestion{"hello", 2},
		index.Suggestion{"jello world", 3},
	))

	suggs, err := ac.Suggest("hel", 10, false)
	assert.NoError(t, err)
	assert.Len(t, suggs, 2)

	suggs, err = ac.Suggest("hel", 10, true)
	assert.NoError(t, err)
	assert.Len(t, suggs, 3)
}

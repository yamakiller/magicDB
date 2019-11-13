package magicDB

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yamakiller/magicWeb/util"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type mongoClient struct {
	c       *mongo.Client
	db      *mongo.Database
	timeSec int
}

// Connect : Connection mongo db service
func (slf *mongoClient) connect(host []string,
	uri string,
	dbName string,
	poolMax int,
	poolMin int,
	socketTimeSec int,
	timeSec int,
	hbSec int,
	idleSec int) error {
	slf.timeSec = timeSec
	opt := options.ClientOptions{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeSec)*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx,
		opt.ApplyURI(uri),
		opt.SetHosts(host),
		opt.SetHeartbeatInterval(time.Duration(hbSec)*time.Second),
		opt.SetMaxPoolSize(uint64(poolMax)),
		opt.SetMinPoolSize(uint64(poolMin)),
		opt.SetMaxConnIdleTime(time.Duration(idleSec)*time.Second),
		opt.SetSocketTimeout(time.Duration(socketTimeSec)*time.Second))
	if err != nil {
		return err
	}

	slf.db = client.Database(dbName)
	if slf.db == nil {
		client.Disconnect(ctx)
		return fmt.Errorf("mongoDB Database %s does not exist", dbName)
	}

	slf.c = client

	return nil
}

func (slf *mongoClient) close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.timeSec)*time.Second)
	defer cancel()
	slf.c.Disconnect(ctx)
	slf.c = nil
	slf.db = nil
}

//MongoDB desc:
//@struct MongoDB desc: Mongo DB Object
//@member ([]*mongoClient) mongo client array
type MongoDB struct {
	cs   []*mongoClient
	size int
	mx   sync.Mutex

	Deplay    *MongoDeplay
	MinClient int
	MaxClient int
}

//Init desc
//@method Init desc: initialization mongo db
//@return (error) initialization fail informat
func (slf *MongoDB) Init() error {
	slf.mx.Lock()
	defer slf.mx.Unlock()
	for i := 0; i < slf.MinClient; i++ {
		mgc := &mongoClient{}
		err := mgc.connect(slf.Deplay.Host,
			slf.Deplay.URI,
			slf.Deplay.DBName,
			slf.Deplay.MaxPoolSize,
			slf.Deplay.MinPoolSize,
			slf.Deplay.SocketTimeout,
			slf.Deplay.TimeOut,
			slf.Deplay.HeartbeatInterval,
			slf.Deplay.IdleTime)
		util.AssertEmpty(err != nil, err.Error())
		slf.cs = append(slf.cs, mgc)
		slf.size++
	}

	return nil
}

//Close desc
//@method Close desc: close mongo db
func (slf *MongoDB) Close() {
	for {
		slf.mx.Lock()
		if slf.size == 0 {
			slf.mx.Unlock()
			break
		}

		n := len(slf.cs)
		if n == 0 {
			slf.mx.Unlock()
			time.Sleep(time.Millisecond * 5)
			continue
		}

		for _, v := range slf.cs {
			v.close()
		}

		slf.cs = slf.cs[n:]
		slf.size -= n
	}
}

func (slf *MongoDB) freeClient(free *mongoClient) {
	slf.mx.Lock()
	defer slf.mx.Unlock()
	slf.cs = append(slf.cs, free)
}

func (slf *MongoDB) getClient() (*mongoClient, error) {
	slf.mx.Lock()
	defer slf.mx.Unlock()
	if len(slf.cs) == 0 {
		if slf.size < slf.MaxClient {
			mgc := &mongoClient{}
			err := mgc.connect(slf.Deplay.Host,
				slf.Deplay.URI,
				slf.Deplay.DBName,
				slf.Deplay.MaxPoolSize,
				slf.Deplay.MinPoolSize,
				slf.Deplay.SocketTimeout,
				slf.Deplay.TimeOut,
				slf.Deplay.HeartbeatInterval,
				slf.Deplay.IdleTime)
			if err != nil {
				return nil, err
			}
			slf.size++
			return mgc, nil
		}
		return nil, fmt.Errorf("mongoDB dbpooling is fulled")
	}

	client := slf.cs[0]
	slf.cs = slf.cs[1:]

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	if err := client.c.Ping(ctx, readpref.Primary()); err != nil {
		slf.size--
		client.close()
		return nil, err
	}

	return client, nil
}

//InsertOne desc
//@method InsertOne desc: Insert a piece of data
//@param (string) set/table name
//@param (interface{}) data
//@return (interface{}) insert result
//@return (error) insert fail
func (slf *MongoDB) InsertOne(name string, document interface{}) (interface{}, error) {
	client, err := slf.getClient()
	if err != nil {
		return nil, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r, rerr := client.db.Collection(name).InsertOne(ctx, document)
	if rerr != nil {
		return nil, rerr
	}

	return r.InsertedID, nil
}

//InsertMany desc
//@method InsertMany desc: Insert multiple pieces of data
//@param (string) set/table name
//@param ([]interface{}) more data
//@return (interface{}) insert result
//@return (error) insert fail
func (slf *MongoDB) InsertMany(name string, document []interface{}) ([]interface{}, error) {
	client, err := slf.getClient()
	if err != nil {
		return nil, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r, rerr := client.db.Collection(name).InsertMany(ctx, document)
	if rerr != nil {
		return nil, rerr
	}
	return r.InsertedIDs, nil
}

//FindOne desc
//@method FindOne desc: Query a piece of data
//@param (string) set/table name
//@param (interface{}) filter
//@param (interface{}) out result
//@return (error) Return error
func (slf *MongoDB) FindOne(name string, filter interface{}, out interface{}) error {
	client, err := slf.getClient()
	if err != nil {
		return err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r := client.db.Collection(name).FindOne(ctx, filter)
	if derr := r.Decode(out); derr != nil {
		return derr
	}

	return nil
}

//Find desc
//@method Find desc: Query multiple data
//@param (string) set/table name
//@param (interface{}) filter
//@param (interface{})
//@return ([]interface{}) Return result
//@return (error) Return error
func (slf *MongoDB) Find(name string, filter interface{}, decode interface{}) ([]interface{}, error) {
	client, err := slf.getClient()
	if err != nil {
		return nil, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r, rerr := client.db.Collection(name).Find(ctx, filter)
	if rerr != nil {
		return nil, rerr
	}

	defer r.Close(ctx)
	ary := make([]interface{}, 0, 4)
	for r.Next(ctx) {
		if derr := r.Decode(&decode); derr != nil {
			return nil, derr
		}

		ary = append(ary, decode)
	}

	return ary, nil
}

//UpdateOne desc
//@method UpdateOne desc: update a piece of data
//@param (string) set/table name
//@param (interface{}) filter
//@param (interface{}) update informat
//@return (int64) match of number
//@return (int64) modify of number
//@return (int64) update of number
//@return (interface{}) update id
//@return (error)
func (slf *MongoDB) UpdateOne(name string, filter interface{}, update interface{}) (int64, int64, int64, interface{}, error) {
	client, err := slf.getClient()
	if err != nil {
		return 0, 0, 0, nil, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r, rerr := client.db.Collection(name).UpdateOne(ctx, filter, update)
	if rerr != nil {
		return 0, 0, 0, nil, rerr
	}

	return r.MatchedCount, r.ModifiedCount, r.UpsertedCount, r.UpsertedID, nil
}

//UpdateMany desc
//@method UpdateMany desc: update multiple data
//@param (string) set/table name
//@param (interface{}) filter
//@param (interface{}) update informat
//@return (int64) match of number
//@return (int64) modify of number
//@return (int64) update of number
//@return (interface{}) update id
//@return (error)
func (slf *MongoDB) UpdateMany(name string, filter interface{}, update interface{}) (int64, int64, int64, interface{}, error) {
	client, err := slf.getClient()
	if err != nil {
		return 0, 0, 0, nil, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r, rerr := client.db.Collection(name).UpdateMany(ctx, filter, update)
	if rerr != nil {
		return 0, 0, 0, nil, rerr
	}

	return r.MatchedCount, r.ModifiedCount, r.UpsertedCount, r.UpsertedID, nil
}

//ReplaceOne desc
//@method ReplaceOne desc: replace a piece of data
//@param (string) set/table name
//@param (interface{}) filter
//@param (interface{}) update informat
//@return (int64) match of number
//@return (int64) modify of number
//@return (int64) update of number
//@return (interface{}) update id
//@return (error)
func (slf *MongoDB) ReplaceOne(name string, filter interface{}, replacement interface{}) (int64, int64, int64, interface{}, error) {
	client, err := slf.getClient()
	if err != nil {
		return 0, 0, 0, nil, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r, rerr := client.db.Collection(name).ReplaceOne(ctx, filter, replacement)

	if rerr != nil {
		return 0, 0, 0, nil, rerr
	}

	return r.MatchedCount, r.ModifiedCount, r.UpsertedCount, r.UpsertedID, nil
}

//DeleteOne desc
//@method DeleteOne desc: delete a piece of data
//@param (string) set/table name
//@param (interface{}) filter
//@return (int64) delete of number
//@return (error)
func (slf *MongoDB) DeleteOne(name string, filter interface{}) (int64, error) {
	client, err := slf.getClient()
	if err != nil {
		return 0, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r, rerr := client.db.Collection(name).DeleteOne(ctx, filter)
	if rerr != nil {
		return 0, rerr
	}

	return r.DeletedCount, nil
}

//DeleteMany desc
//@method DeleteMany desc: Delete multiple pieces of data
//@param (string) set/table name
//@param (interface{}) filter
//@return (int64) delete of number
//@return (error)
func (slf *MongoDB) DeleteMany(name string, filter interface{}) (int64, error) {
	client, err := slf.getClient()
	if err != nil {
		return 0, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r, rerr := client.db.Collection(name).DeleteMany(ctx, filter)
	if rerr != nil {
		return 0, rerr
	}

	return r.DeletedCount, nil
}

//FindOneAndDelete desc
//@method FindOneAndDelete desc: find a piece of data and delete
//@param (string) set/table name
//@param (interface{}) filter
//@param (out interface{}) One piece of data found
//@return (error)
func (slf *MongoDB) FindOneAndDelete(name string, filter interface{}, out interface{}) error {
	client, err := slf.getClient()
	if err != nil {
		return err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r := client.db.Collection(name).FindOneAndDelete(ctx, filter)

	if derr := r.Decode(out); derr != nil {
		return derr
	}

	return nil
}

//FindOneAndUpdate desc
//@method FindOneAndUpdate desc: find a piece of data and update
//@param (string) set/table name
//@param (interface{}) filter
//@param (interface{}) data to be updated
//@param (out interface{}) One piece of data found
//@return (error)
func (slf *MongoDB) FindOneAndUpdate(name string, filter interface{}, update interface{}, out interface{}) error {
	client, err := slf.getClient()
	if err != nil {
		return err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r := client.db.Collection(name).FindOneAndUpdate(ctx, filter, update)
	if derr := r.Decode(out); derr != nil {
		return derr
	}

	return nil
}

//FindOneAndReplace desc
//@method FindOneAndReplace desc: find a piece of data and replace
//@param (string) set/table name
//@param (interface{}) filter
//@param (interface{}) data to be replace
//@param (out interface{}) One piece of data found
//@return (error)
func (slf *MongoDB) FindOneAndReplace(name string, filter interface{}, replacement interface{}, out interface{}) error {
	client, err := slf.getClient()
	if err != nil {
		return err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r := client.db.Collection(name).FindOneAndReplace(ctx, filter, replacement)
	if derr := r.Decode(out); derr != nil {
		return derr
	}

	return nil
}

//Distinct desc
//@method Distinct desc: Find in the specified field
//@param (string) set/table name
//@param (string) field name
//@param (interface{}) filter
//@return ([]interface{}) Return result
//@return (error)
func (slf *MongoDB) Distinct(name string, fieldName string, filter interface{}) ([]interface{}, error) {
	client, err := slf.getClient()
	if err != nil {
		return nil, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	r, rerr := client.db.Collection(name).Distinct(ctx, fieldName, filter)
	if rerr != nil {
		return nil, rerr
	}

	return r, nil
}

//Drop desc:
//@method Drop desc: Delete set/table
//@param  (string) set/table name
//@return (error)
func (slf *MongoDB) Drop(name string) error {
	client, err := slf.getClient()
	if err != nil {
		return err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	return client.db.Collection(name).Drop(ctx)
}

//CountDocuments desc:
//@method CountDocuments desc: Return the total number of documents
//@param (string) set/table name
//@param (interface{}) filter
//@return (int64) a number
//@return (error)
func (slf *MongoDB) CountDocuments(name string, filter interface{}) (int64, error) {
	client, err := slf.getClient()
	if err != nil {
		return 0, err
	}
	defer slf.freeClient(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(slf.Deplay.TimeOut)*time.Second)
	defer cancel()

	return client.db.Collection(name).CountDocuments(ctx, filter)
}

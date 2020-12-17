package book

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	m "github.com/huantingwei/go/models"
	"github.com/huantingwei/go/util"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

const layoutISO = "2006-01-02 15:04:05"

type Service struct {
	client         *mongo.Client
	bookCollection *mongo.Collection
	noteCollection *mongo.Collection
}

func NewService(r *gin.RouterGroup, db util.Database) {
	s := &Service{
		client:         db.Client,
		bookCollection: db.Handle.Collection("book"),
		noteCollection: db.Handle.Collection("note"),
	}
	r.GET("/test", s.transaction)

	book := r.Group("/book")
	{
		book.GET("", s.ListBook)
		book.GET("/:bookid", s.GetBook)
		book.POST("", s.AddBook)
		book.DELETE("", s.DeleteBook)
		book.POST("/:bookid", s.EditBook)
	}

	note := r.Group("/note")
	{
		note.GET("", s.ListNoteByBook)
		note.GET("/:noteid", s.GetNote)
		note.POST("", s.AddNote)
		note.DELETE("", s.DeleteNote)
		note.POST("/:noteid", s.EditNote)
	}

}

// ListBook enumerate all books
// request: GET "/api/v1/book"
// response: [ {...BOOK_1}, {...BOOK_2}]
func (s *Service) ListBook(c *gin.Context) {
	query := map[string]string{
		// "id":        c.Query("id"),
		"title":  c.Query("title"),
		"author": c.Query("author"),
		"status": c.Query("status"),
		// "startTime": c.Query("startTime"),
		// "endTime":   c.Query("endTime"),
	}

	// construct filter
	// bson.D{{"name", "hello"}} => find data with name == hello
	// check if any filter value exists with `listAll`
	var f bson.D
	listAll := true
	for k, v := range query {
		if v != "" {
			f = append(f, bson.E{Key: k, Value: v})
			listAll = false
		}
	}
	// convert type of filter
	var filter interface{}
	if listAll == true {
		// if no filter value, use bson.M{}
		filter = bson.M{}
	} else {
		// otherwise, use bson.D{bson.E{}}
		// e.g. bson.D{{"name", "hello"}}
		filter = f
	}

	fmt.Printf("filter: %s\n", filter)

	cursor, err := s.bookCollection.Find(context.TODO(), filter)
	if err != nil {
		log.Printf("Could not get books with filter %v.\nError: %v", filter, err)
		util.ResponseError(c, err)
		return
	}

	var books []m.Book
	if err = cursor.All(context.TODO(), &books); err != nil {
		log.Printf("Could not decode books.\nError: %v", err)
		util.ResponseError(c, err)
		return
	}

	util.ResponseSuccess(c, books)
}

// GetBook retrieves the book with the given bookid
// return the information of the book
// request: GET "/api/v1/book/:bookid"
// response: {...BOOK}
func (s *Service) GetBook(c *gin.Context) {

	id := c.Param("bookid")

	// convert to primitiv.ObjectID
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Println("Invalid id")
		util.ResponseError(c, err)
		return
	}

	// get book
	var book m.Book
	res := s.bookCollection.FindOne(context.TODO(), bson.M{"id": oid})
	res.Decode(&book)

	// check if the book exist
	// if no, mongodb will return primitive.NilObjectID
	if book.ID == primitive.NilObjectID {
		util.ResponseError(c, fmt.Errorf("Book %v does not exist\n", id))
		return
	}

	util.ResponseSuccess(c, book)

}

// AddBook receives all information of a book and insert one in db
// returns the id of the newly created book
// request: POST "/api/v1/book" form-data: {...BOOK}
// response: `string(primitive.ObjectID)` BOOK_ID
func (s *Service) AddBook(c *gin.Context) {

	var book m.Book
	c.ShouldBindJSON(&book)

	// self generated id field
	book.ID = primitive.NewObjectID()
	book.Notes = make([]primitive.ObjectID, 0)

	_, err := s.bookCollection.InsertOne(context.TODO(), book)
	if err != nil {
		log.Printf("Could not create Book: %v", err)
		util.ResponseError(c, err)
		return
	}

	util.ResponseSuccess(c, book.ID)

}

// DeleteBook delete the book with the given id and all its notes
// request: DELETE "/api/v1/book" form-data: {id: ID}
func (s *Service) DeleteBook(c *gin.Context) {
	id := c.PostForm("id")
	// convert to primitiv.ObjectID
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Println("Invalid id")
		util.ResponseError(c, err)
		return
	}

	// delete notes where note.bookID = bookID
	if _, err := s.noteCollection.DeleteMany(context.TODO(), bson.D{{Key: "bookid", Value: oid}}); err != nil {
		log.Printf("Could not delete book %v's notes.\nError: %v", oid, err)
		util.ResponseError(c, err)
		return
	}

	// delete Book
	res, err := s.bookCollection.DeleteOne(context.TODO(), bson.M{"id": oid})
	if err != nil {
		// TODO: restore all deleted notes
		log.Printf("Could not delete book %v.\nError: %v", oid, err)
		util.ResponseError(c, err)
		return
	}

	util.ResponseSuccess(c, int(res.DeletedCount))

}

// EditBook edit the book with the given id
// request: POST "/api/v1/book/:bookid" form-data: {...FIELD(s)}
// response: {...EDITED_BOOK}
func (s *Service) EditBook(c *gin.Context) {

	fields := make(map[string]interface{})
	c.ShouldBindJSON(&fields)

	// convert to primitive.ObjectID
	oid, err := strIdToPrimitive(fields["id"])
	if err != nil {
		util.ResponseError(c, fmt.Errorf("Invalid id"))
		return
	}
	fields["id"] = oid

	// if str, ok := fields["id"].(string); ok {
	// 	oid, err := primitive.ObjectIDFromHex(str)
	// 	if err != nil {
	// 		log.Println("Invalid id")
	// 		util.ResponseError(c, err)
	// 		return
	// 	}
	// 	fields["id"] = oid
	// } else {
	// 	log.Println("Invalid id")
	// 	util.ResponseError(c, fmt.Errorf("Invalid id"))
	// 	return
	// }

	var updateFields bson.D
	for k, v := range fields {
		if v != "" {
			updateFields = append(updateFields, bson.E{Key: k, Value: v})
		}
	}
	// TODO: the return data is not not updated
	var updatedDocument bson.M
	err = s.bookCollection.FindOneAndUpdate(
		context.TODO(),
		bson.D{{Key: "id", Value: fields["id"]}},
		bson.D{
			{Key: "$set", Value: updateFields},
		},
	).Decode(&updatedDocument)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			util.ResponseError(c, fmt.Errorf("id %v does not match any book\n", fields["id"]))
			return
		}
		log.Printf("Could not edit book %v.\nError: %v", fields["id"], err)
		util.ResponseError(c, err)
		return
	}
	// TODO: updatedDocument is not updated
	util.ResponseSuccess(c, updatedDocument)
}

// ListNoteByBook enumerates all notes of a book
// request: GET "/api/v1/note"
// response: [ {...NOTE_1}, {...NOTE_2}]
func (s *Service) ListNoteByBook(c *gin.Context) {
	id := c.Query("bookid")
	// convert to primitiv.ObjectID
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Println("Invalid id")
		util.ResponseError(c, err)
		return
	}

	book := getBook(oid, s)
	// check if the book exist
	// if no, mongodb will return primitive.NilObjectID
	if book.ID == primitive.NilObjectID {
		util.ResponseError(c, fmt.Errorf("Book %v does not exist\n", id))
		return
	}

	var notes []m.Note
	noteIDs := book.Notes
	for _, noteID := range noteIDs {
		var note m.Note
		res := s.noteCollection.FindOne(context.TODO(), bson.M{"id": noteID})
		res.Decode(&note)
		notes = append(notes, note)
	}

	util.ResponseSuccess(c, notes)
}

// request POST "/api/v1/note/:noteid"
// response: {...}
func (s *Service) GetNote(c *gin.Context) {
	id := c.Param("noteid")
	// convert to primitiv.ObjectID
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Println("Invalid id")
		util.ResponseError(c, err)
		return
	}

	var note m.Note
	res := s.noteCollection.FindOne(context.TODO(), bson.M{"id": oid})
	res.Decode(&note)

	// check if the book exist
	// if no, mongodb will return primitive.NilObjectID
	if note.ID == primitive.NilObjectID {
		util.ResponseError(c, fmt.Errorf("Book %v does not exist\n", id))
		return
	}

	util.ResponseSuccess(c, note)

}

// request POST "/api/v1/note" json: {...}
// response: { Success: true/false, Data: NOTE_ID }
func (s *Service) AddNote(c *gin.Context) {

	var note m.Note
	c.ShouldBindJSON(&note)
	note.ID = primitive.NewObjectID()
	note.CreateTime = time.Now()

	// append the noteID to the Book's `notes`
	if _, err := s.bookCollection.UpdateOne(context.TODO(),
		bson.M{"id": note.BookID},
		bson.D{{Key: "$push", Value: bson.D{{Key: "notes", Value: note.ID}}}}); err != nil {
		log.Printf("Could not append note to book: %v", err)
		util.ResponseError(c, err)
		return
	}

	// insert note into noteCollection
	if _, err := s.noteCollection.InsertOne(context.TODO(), note); err != nil {
		log.Printf("Could not create Note: %v", err)
		deleteOK := deleteNote(note.BookID, note.ID, s)
		if deleteOK {
			log.Printf("Remove this note from book %v\n", note.BookID)
		}
		util.ResponseError(c, err)
		return
	}

	util.ResponseSuccess(c, note.ID)

}

// request DELETE "/api/v1/note" json: {...}
// response: { Success: true/false, Data: # of deleted note }
func (s *Service) DeleteNote(c *gin.Context) {

	var tempNote m.Note
	c.ShouldBindJSON(&tempNote)

	var note m.Note
	// get note, to get bookID
	res := s.noteCollection.FindOne(context.TODO(), bson.M{"id": tempNote.ID})
	res.Decode(&note)
	bookID := note.BookID

	// delete note from Book.notes
	isDeleted := deleteNote(bookID, tempNote.ID, s)
	if !isDeleted {
		util.ResponseError(c, fmt.Errorf("Cannot remove note %v from book %v\n", tempNote.ID, bookID))
		return
	}

	// delete note
	delNoteRes, err := s.noteCollection.DeleteOne(context.TODO(), bson.M{"id": tempNote.ID})
	if err != nil {
		log.Printf("Could not delete Note.\nError: %v", err)
		util.ResponseError(c, err)
		return
	}

	util.ResponseSuccess(c, int(delNoteRes.DeletedCount))
}

func (s *Service) EditNote(c *gin.Context) {

	fields := make(map[string]interface{})
	c.ShouldBindJSON(&fields)

	// convert to primitive.ObjectID
	oid, err := strIdToPrimitive(fields["id"])
	if err != nil {
		util.ResponseError(c, fmt.Errorf("Invalid id"))
		return
	}
	fields["id"] = oid

	var updateFields bson.D
	for k, v := range fields {
		if v != "" {
			updateFields = append(updateFields, bson.E{Key: k, Value: v})
		}
	}

	res, err := s.noteCollection.UpdateOne(
		context.TODO(),
		bson.M{"id": fields["id"]},
		bson.D{
			{Key: "$set", Value: updateFields},
		},
	)
	if err != nil {
		log.Printf("Could not edit note %v.\nError: %v", fields["id"], err)
		util.ResponseError(c, err)
		return
	}
	util.ResponseSuccess(c, int(res.ModifiedCount))
}

func (s *Service) transaction(c *gin.Context) {
	wc := writeconcern.New(writeconcern.WMajority())
	rc := readconcern.Snapshot()
	txnOpts := options.Transaction().SetWriteConcern(wc).SetReadConcern(rc)
	session, err := s.client.StartSession()
	if err != nil {
		fmt.Printf("can't start, err: %v\n", err)
		panic(err)
	}
	defer session.EndSession(context.Background())
	err = mongo.WithSession(context.Background(), session, func(sessionContext mongo.SessionContext) error {
		if err = session.StartTransaction(txnOpts); err != nil {
			fmt.Printf("1st: %v\n", err)
			return err
		}
		result, err := s.bookCollection.InsertOne(
			sessionContext,
			m.Book{
				Title:       "A Transaction Episode for the Ages",
				Author:      "hello",
				Status:      1,
				Description: "description",
				ID:          primitive.NewObjectID(),
				Notes:       make([]primitive.ObjectID, 0),
			},
		)
		if err != nil {
			fmt.Printf("2nd: %v\n", err)
			return err
		}
		fmt.Println(result.InsertedID)
		result, err = s.bookCollection.InsertOne(
			sessionContext,
			m.Book{
				Title:       "Transactions for All",
				Author:      "second",
				Status:      1,
				Description: "description",
				ID:          primitive.NewObjectID(),
				Notes:       make([]primitive.ObjectID, 0),
			},
		)
		if err != nil {
			fmt.Printf("3rd: %v\n", err)
			return err
		}
		if err = session.CommitTransaction(sessionContext); err != nil {
			fmt.Printf("4th: %v\n", err)
			return err
		}
		fmt.Println(result.InsertedID)
		return nil
	})
	if err != nil {
		if abortErr := session.AbortTransaction(context.Background()); abortErr != nil {
			fmt.Printf("5th: %v\n", abortErr)
			panic(abortErr)
		}
		fmt.Printf("6th: %v\n", err)
		panic(err)
	}
	util.ResponseSuccess(c, "nice")

}

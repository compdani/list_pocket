package main
import (
  "fmt"
  "github.com/knadh/listmonk/internal/pbdb"
  "github.com/knadh/listmonk/models"
  "github.com/pocketbase/pocketbase"
)
func main(){
 pb:=pocketbase.NewWithConfig(pocketbase.Config{HideStartBanner:true, DefaultDataDir:"pb_data"})
 if err:=pb.Bootstrap(); err!=nil { panic(err) }
 db,err:=pbdb.NewFromPocketBase(pb); if err!=nil { panic(err) }
 var out []models.List
 err=db.Select(&out, `SELECT * FROM lists LIMIT 1`)
 fmt.Printf("err=%v len=%d out=%#v\n", err, len(out), out)
}

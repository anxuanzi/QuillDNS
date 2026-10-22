package auth

import (
 "net/http"
 "net/http/httptest"
 "testing"
)
func TestUnauthenticatedGuard(t *testing.T){
 m:=&Manager{sessions:make(map[[32]byte]session)}
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /",func(w http.ResponseWriter,r *http.Request){w.WriteHeader(200)})
 mux.HandleFunc("GET /api/v1/health",func(w http.ResponseWriter,r *http.Request){w.WriteHeader(200)})
 guard:=m.Guard(mux)
 for _,item:=range []struct{path string;want int}{
  {"/",http.StatusSeeOther},
  {"/api/v1/records",http.StatusUnauthorized},
  {"/api/v1/health",http.StatusOK},
 }{
  rec:=httptest.NewRecorder()
  guard.ServeHTTP(rec,httptest.NewRequest("GET",item.path,nil))
  if rec.Code!=item.want{t.Errorf("%s: got %d want %d",item.path,rec.Code,item.want)}
 }
}

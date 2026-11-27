pragma Singleton
import QtQuick
import Interlinea

// Requests to a connection, with a callback: done(result) gets { ok,
// status, data, error, version }.
QtObject {
    function get(cid, path, done, query) { return call(cid, "GET", path, undefined, done, query) }
    function post(cid, path, body, done, query) { return call(cid, "POST", path, body, done, query) }
    function put(cid, path, body, done, version) { return call(cid, "PUT", path, body, done, undefined, version) }
    function patch(cid, path, body, done, version) { return call(cid, "PATCH", path, body, done, undefined, version) }
    function del(cid, path, done, version) { return call(cid, "DELETE", path, undefined, done, undefined, version) }
    function call(cid, method, path, body, done, query, version) {
        const c = Backend.request(cid, method, path, body === undefined ? null : body, query || {}, version === undefined ? -1 : version)
        if (done) c.done.connect(done)
        return c
    }
    function seg(s) { return Backend.seg(s) }
}

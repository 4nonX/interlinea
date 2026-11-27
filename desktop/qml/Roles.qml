pragma Singleton
import QtQuick
import Interlinea

// Roles and their permissions, as the core names them.
QtObject {
    function allows(role, need) { return Backend.roleAllows(role, need) }
    function text(name) { return Backend.roleText(name) }
    function permissions() { return Backend.permissions() }
    // The roles a place offers (as GET /api/roles lists them; the five
    // built-in ones if that is empty), as [{ id, label }], with current
    // kept where it is no longer defined.
    function choices(defined, current) {
        const ids = (defined && defined.length) ? defined.map(d => d.name) : permissions()
        if (current && ids.indexOf(current) < 0) ids.push(current)
        return ids.map(id => ({ id: id, label: text(id) }))
    }
}

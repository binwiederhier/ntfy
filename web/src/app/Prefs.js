import db from "./db";

class Prefs {
    async setSound(sound) {
        db.prefs.put({key: 'sound', value: sound.toString()});
    }

    async sound() {
        const sound = await db.prefs.get('sound');
        return (sound) ? sound.value : "ding";
    }

    async setMinPriority(minPriority) {
        db.prefs.put({key: 'minPriority', value: minPriority.toString()});
    }

    async minPriority() {
        const minPriority = await db.prefs.get('minPriority');
        return (minPriority) ? Number(minPriority.value) : 1;
    }

    async setDeleteAfter(deleteAfter) {
        db.prefs.put({key:'deleteAfter', value: deleteAfter.toString()});
    }

    async deleteAfter() {
        const deleteAfter = await db.prefs.get('deleteAfter');
        return (deleteAfter) ? Number(deleteAfter.value) : 604800; // Default is one week
    }
}

const prefs = new Prefs();
export default prefs;

class Session {
    store(username, token) {
        localStorage.setItem("user", username);
        localStorage.setItem("token", token);
    }

    reset() {
        localStorage.removeItem("user");
        localStorage.removeItem("token");
    }

    username() {
        return localStorage.getItem("user");
    }

    token() {
        return localStorage.getItem("token");
    }
}

const session = new Session();
export default session;

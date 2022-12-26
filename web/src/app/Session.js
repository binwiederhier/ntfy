import routes from "../components/routes";

class Session {
    store(username, token) {
        localStorage.setItem("user", username);
        localStorage.setItem("token", token);
    }

    reset() {
        localStorage.removeItem("user");
        localStorage.removeItem("token");
    }

    resetAndRedirect(url) {
        this.reset();
        window.location.href = url;
    }

    exists() {
        return this.username() && this.token();
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

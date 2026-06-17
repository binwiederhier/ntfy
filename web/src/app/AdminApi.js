import { fetchOrThrow } from "./errors";
import { withBearerAuth } from "./utils";
import session from "./Session";

const usersUrl = (baseUrl) => `${baseUrl}/v1/users`;
const usersAccessUrl = (baseUrl) => `${baseUrl}/v1/users/access`;

class AdminApi {
  async getUsers() {
    const url = usersUrl(config.base_url);
    console.log(`[AdminApi] Fetching users ${url}`);
    const response = await fetchOrThrow(url, {
      headers: withBearerAuth({}, session.token()),
    });
    return response.json();
  }

  async addUser(username, password, tier) {
    const url = usersUrl(config.base_url);
    const body = { username, password };
    if (tier) {
      body.tier = tier;
    }
    console.log(`[AdminApi] Adding user ${url}`);
    await fetchOrThrow(url, {
      method: "POST",
      headers: withBearerAuth({}, session.token()),
      body: JSON.stringify(body),
    });
  }

  async updateUser(username, password, tier) {
    const url = usersUrl(config.base_url);
    const body = { username };
    if (password) {
      body.password = password;
    }
    if (tier) {
      body.tier = tier;
    }
    console.log(`[AdminApi] Updating user ${url}`);
    await fetchOrThrow(url, {
      method: "PUT",
      headers: withBearerAuth({}, session.token()),
      body: JSON.stringify(body),
    });
  }

  async deleteUser(username) {
    const url = usersUrl(config.base_url);
    console.log(`[AdminApi] Deleting user ${url}`);
    await fetchOrThrow(url, {
      method: "DELETE",
      headers: withBearerAuth({}, session.token()),
      body: JSON.stringify({ username }),
    });
  }

  async allowAccess(username, topic, permission) {
    const url = usersAccessUrl(config.base_url);
    console.log(`[AdminApi] Allowing access ${url}`);
    await fetchOrThrow(url, {
      method: "PUT",
      headers: withBearerAuth({}, session.token()),
      body: JSON.stringify({ username, topic, permission }),
    });
  }

  async resetAccess(username, topic) {
    const url = usersAccessUrl(config.base_url);
    console.log(`[AdminApi] Resetting access ${url}`);
    await fetchOrThrow(url, {
      method: "DELETE",
      headers: withBearerAuth({}, session.token()),
      body: JSON.stringify({ username, topic }),
    });
  }
}

const adminApi = new AdminApi();
export default adminApi;


import { fetchOrThrow } from "./errors";
import { adminTiersUrl, usersAccessUrl, usersReservationsUrl, usersUrl, withBearerAuth } from "./utils";
import session from "./Session";

// Header with the total number of rows of a paginated list response, see server/server_admin.go
const totalCountHeader = "X-Total-Count";

// Special tier value to remove a user's tier, see server/server_admin.go
export const TIER_RESET = "-";

const withPagination = (url, { search, limit, offset }) => {
  const params = new URLSearchParams();
  if (search) {
    params.set("search", search);
  }
  if (limit) {
    params.set("limit", limit);
  }
  if (offset) {
    params.set("offset", offset);
  }
  const query = params.toString();
  return query ? `${url}?${query}` : url;
};

class AdminApi {
  async getUsers(pagination) {
    return this.getPage(withPagination(usersUrl(config.base_url), pagination));
  }

  async addUser(username, password, tier) {
    const body = { username, password };
    if (tier) {
      body.tier = tier;
    }
    await this.send("POST", usersUrl(config.base_url), body);
  }

  async updateUser(username, password, tier) {
    const body = { username };
    if (password) {
      body.password = password;
    }
    if (tier) {
      body.tier = tier;
    }
    await this.send("PUT", usersUrl(config.base_url), body);
  }

  async deleteUser(username) {
    await this.send("DELETE", usersUrl(config.base_url), { username });
  }

  async getAccess(pagination) {
    return this.getPage(withPagination(usersAccessUrl(config.base_url), pagination));
  }

  async allowAccess(username, topic, permission) {
    await this.send("PUT", usersAccessUrl(config.base_url), { username, topic, permission });
  }

  async resetAccess(username, topic) {
    await this.send("DELETE", usersAccessUrl(config.base_url), { username, topic });
  }

  async getReservations(pagination) {
    return this.getPage(withPagination(usersReservationsUrl(config.base_url), pagination));
  }

  async updateReservation(username, topic, everyone) {
    await this.send("PUT", usersReservationsUrl(config.base_url), { username, topic, everyone });
  }

  async deleteReservation(username, topic) {
    await this.send("DELETE", usersReservationsUrl(config.base_url), { username, topic });
  }

  async getTiers() {
    const url = adminTiersUrl(config.base_url);
    console.log(`[AdminApi] Fetching tiers ${url}`);
    const response = await fetchOrThrow(url, {
      headers: withBearerAuth({}, session.token()),
    });
    return response.json();
  }

  async addTier(tier) {
    await this.send("POST", adminTiersUrl(config.base_url), tier);
  }

  async updateTier(tier) {
    await this.send("PUT", adminTiersUrl(config.base_url), tier);
  }

  async deleteTier(code) {
    await this.send("DELETE", adminTiersUrl(config.base_url), { code });
  }

  // getPage fetches a paginated list, and returns the rows along with the total number of matching rows
  async getPage(url) {
    console.log(`[AdminApi] Fetching ${url}`);
    const response = await fetchOrThrow(url, {
      headers: withBearerAuth({}, session.token()),
    });
    const rows = await response.json();
    const total = parseInt(response.headers.get(totalCountHeader) ?? "", 10);
    return { rows, total: Number.isNaN(total) ? rows.length : total };
  }

  async send(method, url, body) {
    console.log(`[AdminApi] ${method} ${url}`);
    await fetchOrThrow(url, {
      method,
      headers: withBearerAuth({}, session.token()),
      body: JSON.stringify(body),
    });
  }
}

const adminApi = new AdminApi();
export default adminApi;

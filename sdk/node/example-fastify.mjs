/**
 * Fastify integration manual test script
 *
 * Usage:
 *   1. Start observrd:  ./server/bin/observrd
 *   2. Run this script: node sdk/node/example-fastify.mjs
 *   3. Open dashboard:  http://localhost:7676
 *   4. Send requests:   curl http://localhost:3000/users/42
 *                       curl http://localhost:3000/error
 *                       curl http://localhost:3000/notfound
 */

import Fastify from "fastify";
import { init, fastifyPlugin } from "./dist/index.mjs";

const client = init({ endpoint: "http://localhost:7676" });
const app = Fastify({ logger: false });

// Register observr plugin
await app.register(fastifyPlugin(client.transport));

// Routes
app.get("/users/:id", async (req, _reply) => {
  return { userId: req.params.id, name: "Alice" };
});

app.get("/error", async (_req, reply) => {
  reply.status(500);
  return { error: "Internal Server Error" };
});

app.get("/notfound", async (_req, reply) => {
  reply.status(404);
  return { error: "Not Found" };
});

app.get("/health", async () => ({ status: "ok" }));

const port = 3000;
await app.listen({ port });
console.log(`Fastify server running at http://localhost:${port}`);
console.log("");
console.log("Try these requests:");
console.log(`  curl http://localhost:${port}/users/42`);
console.log(`  curl http://localhost:${port}/users/99`);
console.log(`  curl http://localhost:${port}/error`);
console.log(`  curl http://localhost:${port}/notfound`);
console.log(`  curl http://localhost:${port}/health`);
console.log("");
console.log("Watch events at: http://localhost:7676");

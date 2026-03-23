"""
Flask example — start observrd first, then run this file.

    $ observrd start
    $ python examples/flask_app.py
    $ curl http://localhost:5000/users
"""

import logging

import observr
from flask import Flask, jsonify

observr.init(service="flask-example")

logger = logging.getLogger(__name__)
app = Flask(__name__)


@app.get("/users")
def get_users():
    logger.info("Fetching user list")
    return jsonify({"users": [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]})


@app.get("/error")
def trigger_error():
    logger.error("Simulated error", extra={"endpoint": "/error"})
    return jsonify({"error": "Something went wrong"}), 500


if __name__ == "__main__":
    app.run(debug=True)

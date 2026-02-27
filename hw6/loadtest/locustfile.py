import random
from locust import FastHttpUser, HttpUser, between, task

TERMS = ["alpha", "electronics", "books", "home", "sports", "beauty", "toy", "atlas"]


class SearchFastUser(FastHttpUser):
    wait_time = between(0.0, 0.05)

    @task(5)
    def search(self):
        term = random.choice(TERMS)
        self.client.get(f"/products/search?q={term}", name="GET /products/search")

    @task(1)
    def health(self):
        self.client.get("/health", name="GET /health")


class SearchHttpUser(HttpUser):
    wait_time = between(0.0, 0.05)

    @task(5)
    def search(self):
        term = random.choice(TERMS)
        self.client.get(f"/products/search?q={term}", name="GET /products/search")

    @task(1)
    def health(self):
        self.client.get("/health", name="GET /health")

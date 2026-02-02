from locust import HttpUser, task, between

class WebsiteUser(HttpUser):
    wait_time = between(0.1, 0.5)

    @task(3)
    def get_albums(self):
        self.client.get("/albums")

    @task(1)
    def post_album(self):
        self.client.post(
            "/albums",
            json={"id": "12345", "title": "test", "artist": "locust", "price": 1.23},
            headers={"Content-Type": "application/json"},
        )

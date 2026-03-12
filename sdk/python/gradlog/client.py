"""HTTP client for communicating with the Gradlog API."""

from __future__ import annotations

import os
from typing import Any, BinaryIO
from urllib.parse import urljoin

import requests

from gradlog.exceptions import (
    AuthenticationError,
    GradlogError,
    NotFoundError,
    ServerError,
    ValidationError,
)
from gradlog.project import Project
from gradlog.version import __version__


class Client:
    """
    Main client for interacting with the Gradlog API.
    
    Can be used for high-level experiment tracking or raw API access.
    
    Args:
        host: The Gradlog server URL. Defaults to GRADLOG_HOST env var.
        api_key: Your API key. Defaults to GRADLOG_API_KEY env var.
        timeout: Request timeout in seconds. Defaults to 30.
    
    Examples:
        # High-level usage
        client = Client(host="https://gradlog.example.com", api_key="gl_...")
        project = client.get_or_create_project("my-project")
        
        # Raw API usage
        response = client.get("/api/v1/projects")
    """

    def __init__(
        self,
        host: str | None = None,
        api_key: str | None = None,
        timeout: int = 30,
    ):
        self.host = host or os.environ.get("GRADLOG_HOST", "http://localhost:8080")
        self.api_key = api_key or os.environ.get("GRADLOG_API_KEY")
        self.timeout = timeout
        
        # Ensure host doesn't end with a slash.
        self.host = self.host.rstrip("/")
        
        # Create a session for connection pooling.
        self._session = requests.Session()
        if self.api_key:
            self._session.headers["Authorization"] = f"ApiKey {self.api_key}"
        self._session.headers["Content-Type"] = "application/json"
        self._session.headers["User-Agent"] = f"gradlog-python/{__version__}"

    def _handle_response(self, response: requests.Response) -> requests.Response:
        """Handle API response and raise appropriate exceptions."""
        if response.status_code == 401:
            raise AuthenticationError(
                "Authentication failed. Check your API key.",
                status_code=401,
            )
        elif response.status_code == 404:
            raise NotFoundError(
                "Resource not found.",
                status_code=404,
            )
        elif response.status_code == 400:
            try:
                error_msg = response.json().get("error", "Validation error")
            except Exception:
                error_msg = "Validation error"
            raise ValidationError(error_msg, status_code=400)
        elif response.status_code >= 500:
            raise ServerError(
                f"Server error: {response.status_code}",
                status_code=response.status_code,
            )
        elif not response.ok:
            raise GradlogError(
                f"Request failed: {response.status_code}",
                status_code=response.status_code,
            )
        return response

    def _url(self, path: str) -> str:
        """Build full URL from path."""
        return urljoin(self.host + "/", path.lstrip("/"))

    # Raw HTTP methods for direct API access.
    
    def get(self, path: str, **kwargs: Any) -> requests.Response:
        """Make a GET request to the API."""
        response = self._session.get(
            self._url(path),
            timeout=self.timeout,
            **kwargs,
        )
        return self._handle_response(response)

    def post(self, path: str, json: dict[str, Any] | None = None, **kwargs: Any) -> requests.Response:
        """Make a POST request to the API."""
        response = self._session.post(
            self._url(path),
            json=json,
            timeout=self.timeout,
            **kwargs,
        )
        return self._handle_response(response)

    def patch(self, path: str, json: dict[str, Any] | None = None, **kwargs: Any) -> requests.Response:
        """Make a PATCH request to the API."""
        response = self._session.patch(
            self._url(path),
            json=json,
            timeout=self.timeout,
            **kwargs,
        )
        return self._handle_response(response)

    def delete(self, path: str, **kwargs: Any) -> requests.Response:
        """Make a DELETE request to the API."""
        response = self._session.delete(
            self._url(path),
            timeout=self.timeout,
            **kwargs,
        )
        return self._handle_response(response)

    def upload(
        self,
        path: str,
        file: BinaryIO,
        filename: str,
        **kwargs: Any,
    ) -> requests.Response:
        """Upload a file to the API."""
        # Remove Content-Type header for multipart.
        headers = dict(self._session.headers)
        del headers["Content-Type"]
        
        response = requests.post(
            self._url(path),
            files={"file": (filename, file)},
            headers=headers,
            timeout=self.timeout,
            **kwargs,
        )
        return self._handle_response(response)

    # High-level API methods.
    
    def list_projects(self) -> list[Project]:
        """List all projects the user has access to."""
        response = self.get("/api/v1/projects")
        return [Project(self, **p) for p in response.json()]

    def get_project(self, project_id: str) -> Project:
        """Get a project by ID."""
        response = self.get(f"/api/v1/projects/{project_id}")
        return Project(self, **response.json())

    def create_project(self, name: str, description: str | None = None) -> Project:
        """Create a new project."""
        response = self.post("/api/v1/projects", json={
            "name": name,
            "description": description,
        })
        return Project(self, **response.json())

    def get_or_create_project(self, name: str, description: str | None = None) -> Project:
        """Get an existing project by name or create a new one."""
        projects = self.list_projects()
        for project in projects:
            if project.name == name:
                return project
        return self.create_project(name, description)

    def get_current_user(self) -> dict[str, Any]:
        """Get the currently authenticated user."""
        response = self.get("/api/v1/auth/me")
        return response.json()

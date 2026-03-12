"""Run model and operations."""

from __future__ import annotations

import os
import time
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from gradlog.client import Client


class Run:
    """
    Represents a Gradlog run (a single ML training or evaluation execution).
    
    Runs track metrics, parameters, tags, and artifacts. Can be used as a
    context manager for automatic status management.
    
    Attributes:
        id: Unique run identifier.
        experiment_id: ID of the parent experiment.
        name: Optional run name.
        status: Run status (running, completed, failed, killed).
        params: Dictionary of parameters.
        tags: Dictionary of tags.
        start_time: Timestamp when the run started.
        end_time: Timestamp when the run ended (if finished).
        created_at: Timestamp when the run was created.
        updated_at: Timestamp when the run was last updated.
    """

    def __init__(self, client: Client, **kwargs: Any):
        self._client = client
        self.id: str = kwargs["id"]
        self.experiment_id: str = kwargs["experiment_id"]
        self.name: str | None = kwargs.get("name")
        self.status: str = kwargs["status"]
        self.params: dict[str, Any] = kwargs.get("params", {})
        self.tags: dict[str, Any] = kwargs.get("tags", {})
        self.start_time: str = kwargs["start_time"]
        self.end_time: str | None = kwargs.get("end_time")
        self.created_at: str = kwargs["created_at"]
        self.updated_at: str = kwargs["updated_at"]

    def __repr__(self) -> str:
        return f"Run(id={self.id!r}, name={self.name!r}, status={self.status!r})"

    def __enter__(self) -> Run:
        """Enter context manager."""
        return self

    def __exit__(self, exc_type: Any, exc_val: Any, exc_tb: Any) -> None:
        """Exit context manager, marking run as completed or failed."""
        if exc_type is not None:
            self.fail()
        else:
            self.complete()

    def _refresh(self, data: dict[str, Any]) -> None:
        """Update instance from response data."""
        self.name = data.get("name")
        self.status = data["status"]
        self.params = data.get("params", {})
        self.tags = data.get("tags", {})
        self.end_time = data.get("end_time")
        self.updated_at = data["updated_at"]

    def update(
        self,
        name: str | None = None,
        status: str | None = None,
        params: dict[str, Any] | None = None,
        tags: dict[str, Any] | None = None,
    ) -> Run:
        """
        Update the run.
        
        Args:
            name: New run name.
            status: New status (running, completed, failed, killed).
            params: Parameters to merge into existing params.
            tags: Tags to merge into existing tags.
        
        Returns:
            Updated run instance.
        """
        payload: dict[str, Any] = {}
        if name is not None:
            payload["name"] = name
        if status is not None:
            payload["status"] = status
        if params is not None:
            payload["params"] = params
        if tags is not None:
            payload["tags"] = tags
        
        response = self._client.patch(f"/api/v1/runs/{self.id}", json=payload)
        self._refresh(response.json())
        return self

    def complete(self) -> Run:
        """Mark the run as completed."""
        return self.update(status="completed")

    def fail(self) -> Run:
        """Mark the run as failed."""
        return self.update(status="failed")

    def kill(self) -> Run:
        """Mark the run as killed."""
        return self.update(status="killed")

    def delete(self) -> None:
        """Delete the run and all associated data."""
        self._client.delete(f"/api/v1/runs/{self.id}")

    # Parameter logging.
    
    def log_param(self, key: str, value: Any) -> Run:
        """
        Log a single parameter.
        
        Args:
            key: Parameter name.
            value: Parameter value.
        
        Returns:
            Self for method chaining.
        """
        return self.update(params={key: value})

    def log_params(self, params: dict[str, Any]) -> Run:
        """
        Log multiple parameters at once.
        
        Args:
            params: Dictionary of parameter names to values.
        
        Returns:
            Self for method chaining.
        """
        return self.update(params=params)

    # Tag logging.
    
    def set_tag(self, key: str, value: Any) -> Run:
        """
        Set a single tag.
        
        Args:
            key: Tag name.
            value: Tag value.
        
        Returns:
            Self for method chaining.
        """
        return self.update(tags={key: value})

    def set_tags(self, tags: dict[str, Any]) -> Run:
        """
        Set multiple tags at once.
        
        Args:
            tags: Dictionary of tag names to values.
        
        Returns:
            Self for method chaining.
        """
        return self.update(tags=tags)

    # Metric logging.
    
    def log_metric(
        self,
        key: str,
        value: float,
        step: int | None = None,
        timestamp: int | None = None,
    ) -> None:
        """
        Log a single metric value.
        
        Args:
            key: Metric name.
            value: Metric value.
            step: Optional step number (e.g., epoch).
            timestamp: Optional Unix timestamp in milliseconds.
        """
        payload: dict[str, Any] = {"key": key, "value": value}
        if step is not None:
            payload["step"] = step
        if timestamp is not None:
            payload["timestamp"] = timestamp
        
        self._client.post(f"/api/v1/runs/{self.id}/metrics", json=payload)

    def log_metrics(
        self,
        metrics: dict[str, float],
        step: int | None = None,
        timestamp: int | None = None,
    ) -> None:
        """
        Log multiple metrics at once (batch operation).
        
        Args:
            metrics: Dictionary of metric names to values.
            step: Optional step number for all metrics.
            timestamp: Optional Unix timestamp for all metrics.
        """
        batch = []
        for key, value in metrics.items():
            metric: dict[str, Any] = {"key": key, "value": value}
            if step is not None:
                metric["step"] = step
            if timestamp is not None:
                metric["timestamp"] = timestamp
            batch.append(metric)
        
        self._client.post(f"/api/v1/runs/{self.id}/metrics/batch", json={"metrics": batch})

    def get_metrics(self, key: str | None = None) -> list[dict[str, Any]]:
        """
        Get logged metrics.
        
        Args:
            key: Optional metric key to filter by.
        
        Returns:
            List of metric records.
        """
        path = f"/api/v1/runs/{self.id}/metrics"
        if key:
            path += f"?key={key}"
        response = self._client.get(path)
        return response.json()

    def get_metric_history(self, key: str) -> dict[str, Any]:
        """
        Get the full history of a specific metric.
        
        Args:
            key: Metric key.
        
        Returns:
            Metric history with all logged values.
        """
        response = self._client.get(f"/api/v1/runs/{self.id}/metrics/{key}/history")
        return response.json()

    def get_latest_metrics(self) -> list[dict[str, Any]]:
        """
        Get the latest value of each metric.
        
        Returns:
            List of the most recent value for each metric key.
        """
        response = self._client.get(f"/api/v1/runs/{self.id}/metrics/latest")
        return response.json()

    # Artifact logging.
    
    def list_artifacts(self) -> list[dict[str, Any]]:
        """
        List all artifacts for this run.
        
        Returns:
            List of artifact records.
        """
        response = self._client.get(f"/api/v1/runs/{self.id}/artifacts")
        return response.json()

    def log_artifact(
        self,
        name: str,
        local_path: str | Path,
        artifact_path: str = "/",
    ) -> dict[str, Any]:
        """
        Upload an artifact file.
        
        For files larger than the chunk size (default 50MB), this will
        automatically use chunked upload to bypass transfer size limits.
        
        Args:
            name: Name for the artifact.
            local_path: Path to the local file to upload.
            artifact_path: Path within the run's artifact directory.
        
        Returns:
            The created artifact record.
        """
        local_path = Path(local_path)
        file_size = local_path.stat().st_size
        
        # Get chunk size from server or use default.
        # For now, use a conservative 50MB default.
        chunk_size = 50 * 1024 * 1024
        
        if file_size <= chunk_size:
            # Simple upload for small files.
            return self._simple_upload(name, local_path, artifact_path)
        else:
            # Chunked upload for large files.
            return self._chunked_upload(name, local_path, artifact_path, file_size, chunk_size)

    def _simple_upload(
        self,
        name: str,
        local_path: Path,
        artifact_path: str,
    ) -> dict[str, Any]:
        """Upload a small file in a single request."""
        with open(local_path, "rb") as f:
            # Remove default Content-Type for multipart upload.
            headers = dict(self._client._session.headers)
            if "Content-Type" in headers:
                del headers["Content-Type"]
            
            response = self._client._session.post(
                self._client._url(f"/api/v1/runs/{self.id}/artifacts/upload"),
                files={"file": (name, f)},
                data={"path": artifact_path},
                headers=headers,
                timeout=self._client.timeout,
            )
            return self._client._handle_response(response).json()

    def _chunked_upload(
        self,
        name: str,
        local_path: Path,
        artifact_path: str,
        file_size: int,
        chunk_size: int,
    ) -> dict[str, Any]:
        """Upload a large file in chunks."""
        # Initialize upload.
        import mimetypes
        content_type = mimetypes.guess_type(str(local_path))[0] or "application/octet-stream"
        
        init_response = self._client.post(
            f"/api/v1/runs/{self.id}/artifacts/init",
            json={
                "path": artifact_path,
                "file_name": name,
                "file_size": file_size,
                "content_type": content_type,
            },
        )
        init_data = init_response.json()
        artifact_id = init_data["artifact_id"]
        total_chunks = init_data["total_chunks"]
        
        # Upload chunks.
        with open(local_path, "rb") as f:
            for chunk_num in range(total_chunks):
                chunk_data = f.read(chunk_size)
                
                # Upload chunk as raw bytes.
                headers = dict(self._client._session.headers)
                headers["Content-Type"] = "application/octet-stream"
                
                response = self._client._session.post(
                    self._client._url(f"/api/v1/artifacts/{artifact_id}/chunks/{chunk_num}"),
                    data=chunk_data,
                    headers=headers,
                    timeout=self._client.timeout * 2,  # Allow more time for chunks.
                )
                self._client._handle_response(response)
        
        # Complete upload.
        complete_response = self._client.post(f"/api/v1/artifacts/{artifact_id}/complete")
        return complete_response.json()

    def download_artifact(
        self,
        artifact_id: str,
        local_path: str | Path,
    ) -> Path:
        """
        Download an artifact file.
        
        Automatically uses chunked download for large files.
        
        Args:
            artifact_id: ID of the artifact to download.
            local_path: Path where the file should be saved.
        
        Returns:
            Path to the downloaded file.
        """
        local_path = Path(local_path)
        
        # Get download info.
        info_response = self._client.get(f"/api/v1/artifacts/{artifact_id}/download-info")
        info = info_response.json()
        
        total_chunks = info["total_chunks"]
        
        if total_chunks <= 1:
            # Simple download.
            response = self._client._session.get(
                self._client._url(f"/api/v1/artifacts/{artifact_id}/download"),
                timeout=self._client.timeout * 2,
            )
            self._client._handle_response(response)
            with open(local_path, "wb") as f:
                f.write(response.content)
        else:
            # Chunked download.
            with open(local_path, "wb") as f:
                for chunk_num in range(total_chunks):
                    response = self._client._session.get(
                        self._client._url(f"/api/v1/artifacts/{artifact_id}/chunks/{chunk_num}"),
                        timeout=self._client.timeout * 2,
                    )
                    self._client._handle_response(response)
                    f.write(response.content)
        
        return local_path

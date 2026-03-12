"""Experiment model and operations."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from gradlog.run import Run

if TYPE_CHECKING:
    from gradlog.client import Client


class Experiment:
    """
    Represents a Gradlog experiment.
    
    Experiments group related runs within a project.
    
    Attributes:
        id: Unique experiment identifier.
        project_id: ID of the parent project.
        name: Experiment name.
        description: Optional experiment description.
        created_at: Timestamp when the experiment was created.
        updated_at: Timestamp when the experiment was last updated.
    """

    def __init__(self, client: Client, **kwargs: Any):
        self._client = client
        self.id: str = kwargs["id"]
        self.project_id: str = kwargs["project_id"]
        self.name: str = kwargs["name"]
        self.description: str | None = kwargs.get("description")
        self.created_at: str = kwargs["created_at"]
        self.updated_at: str = kwargs["updated_at"]

    def __repr__(self) -> str:
        return f"Experiment(id={self.id!r}, name={self.name!r})"

    def update(
        self,
        name: str | None = None,
        description: str | None = None,
    ) -> Experiment:
        """
        Update the experiment.
        
        Args:
            name: New experiment name.
            description: New experiment description.
        
        Returns:
            Updated experiment instance.
        """
        payload = {}
        if name is not None:
            payload["name"] = name
        if description is not None:
            payload["description"] = description
        
        response = self._client.patch(f"/api/v1/experiments/{self.id}", json=payload)
        data = response.json()
        self.name = data["name"]
        self.description = data.get("description")
        self.updated_at = data["updated_at"]
        return self

    def delete(self) -> None:
        """Delete the experiment and all associated runs."""
        self._client.delete(f"/api/v1/experiments/{self.id}")

    def list_runs(self) -> list[Run]:
        """List all runs in this experiment."""
        response = self._client.get(f"/api/v1/experiments/{self.id}/runs")
        return [Run(self._client, **r) for r in response.json()]

    def get_run(self, run_id: str) -> Run:
        """Get a run by ID."""
        response = self._client.get(f"/api/v1/runs/{run_id}")
        return Run(self._client, **response.json())

    def create_run(
        self,
        name: str | None = None,
        params: dict[str, Any] | None = None,
        tags: dict[str, Any] | None = None,
    ) -> Run:
        """
        Create a new run in this experiment.
        
        Args:
            name: Optional run name.
            params: Initial parameters to log.
            tags: Initial tags to add.
        
        Returns:
            The created run.
        """
        payload: dict[str, Any] = {}
        if name is not None:
            payload["name"] = name
        if params is not None:
            payload["params"] = params
        if tags is not None:
            payload["tags"] = tags
        
        response = self._client.post(
            f"/api/v1/experiments/{self.id}/runs",
            json=payload if payload else None,
        )
        return Run(self._client, **response.json())

    def start_run(
        self,
        name: str | None = None,
        params: dict[str, Any] | None = None,
        tags: dict[str, Any] | None = None,
    ) -> Run:
        """
        Start a new run that can be used as a context manager.
        
        The run will automatically be marked as completed (or failed) when
        exiting the context.
        
        Args:
            name: Optional run name.
            params: Initial parameters to log.
            tags: Initial tags to add.
        
        Returns:
            The created run (usable as context manager).
        
        Example:
            with experiment.start_run(name="run-001") as run:
                run.log_metric("loss", 0.5)
        """
        return self.create_run(name=name, params=params, tags=tags)

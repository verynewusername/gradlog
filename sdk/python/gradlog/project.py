"""Project model and operations."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from gradlog.experiment import Experiment

if TYPE_CHECKING:
    from gradlog.client import Client


class Project:
    """
    Represents a Gradlog project.
    
    Projects are top-level containers for experiments and support
    multi-user collaboration.
    
    Attributes:
        id: Unique project identifier.
        name: Project name.
        description: Optional project description.
        owner_id: ID of the project owner.
        created_at: Timestamp when the project was created.
        updated_at: Timestamp when the project was last updated.
    """

    def __init__(self, client: Client, **kwargs: Any):
        self._client = client
        self.id: str = kwargs["id"]
        self.name: str = kwargs["name"]
        self.description: str | None = kwargs.get("description")
        self.owner_id: str = kwargs["owner_id"]
        self.created_at: str = kwargs["created_at"]
        self.updated_at: str = kwargs["updated_at"]

    def __repr__(self) -> str:
        return f"Project(id={self.id!r}, name={self.name!r})"

    def update(
        self,
        name: str | None = None,
        description: str | None = None,
    ) -> Project:
        """
        Update the project.
        
        Args:
            name: New project name.
            description: New project description.
        
        Returns:
            Updated project instance.
        """
        payload = {}
        if name is not None:
            payload["name"] = name
        if description is not None:
            payload["description"] = description
        
        response = self._client.patch(f"/api/v1/projects/{self.id}", json=payload)
        data = response.json()
        self.name = data["name"]
        self.description = data.get("description")
        self.updated_at = data["updated_at"]
        return self

    def delete(self) -> None:
        """Delete the project and all associated experiments and runs."""
        self._client.delete(f"/api/v1/projects/{self.id}")

    def list_experiments(self) -> list[Experiment]:
        """List all experiments in this project."""
        response = self._client.get(f"/api/v1/projects/{self.id}/experiments")
        return [Experiment(self._client, **e) for e in response.json()]

    def get_experiment(self, experiment_id: str) -> Experiment:
        """Get an experiment by ID."""
        response = self._client.get(f"/api/v1/experiments/{experiment_id}")
        return Experiment(self._client, **response.json())

    def create_experiment(self, name: str, description: str | None = None) -> Experiment:
        """
        Create a new experiment in this project.
        
        Args:
            name: Experiment name (must be unique within the project).
            description: Optional experiment description.
        
        Returns:
            The created experiment.
        """
        response = self._client.post(
            f"/api/v1/projects/{self.id}/experiments",
            json={"name": name, "description": description},
        )
        return Experiment(self._client, **response.json())

    def get_or_create_experiment(
        self,
        name: str,
        description: str | None = None,
    ) -> Experiment:
        """
        Get an existing experiment by name or create a new one.
        
        Args:
            name: Experiment name.
            description: Description to use if creating a new experiment.
        
        Returns:
            The experiment (existing or newly created).
        """
        experiments = self.list_experiments()
        for experiment in experiments:
            if experiment.name == name:
                return experiment
        return self.create_experiment(name, description)

    def list_members(self) -> list[dict[str, Any]]:
        """List all members of this project."""
        response = self._client.get(f"/api/v1/projects/{self.id}/members")
        return response.json()

    def add_member(self, email: str, role: str = "member") -> None:
        """
        Add a user to this project.
        
        Args:
            email: Email of the user to add.
            role: Role to assign (admin, member, or viewer).
        """
        self._client.post(
            f"/api/v1/projects/{self.id}/members",
            json={"email": email, "role": role},
        )

    def remove_member(self, user_id: str) -> None:
        """
        Remove a user from this project.
        
        Args:
            user_id: ID of the user to remove.
        """
        self._client.delete(f"/api/v1/projects/{self.id}/members/{user_id}")

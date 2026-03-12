"""Custom exceptions for the Gradlog SDK."""


class GradlogError(Exception):
    """Base exception for all Gradlog SDK errors."""

    def __init__(self, message: str, status_code: int | None = None):
        super().__init__(message)
        self.message = message
        self.status_code = status_code


class AuthenticationError(GradlogError):
    """Raised when authentication fails (invalid API key or token)."""

    pass


class NotFoundError(GradlogError):
    """Raised when a requested resource is not found."""

    pass


class ValidationError(GradlogError):
    """Raised when request validation fails."""

    pass


class ServerError(GradlogError):
    """Raised when the server returns an unexpected error."""

    pass

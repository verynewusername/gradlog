# Gradlog Python SDK

Python SDK for [Gradlog](https://github.com/gradlog/gradlog) ML experiment tracking.

## Installation

```bash
pip install gradlog
```

## Quick Start

```python
import gradlog

# Initialize the client with your API key
client = gradlog.Client(
    host="https://your-gradlog-instance.com",
    api_key="gl_your_api_key"
)

# Create or get a project
project = client.get_or_create_project("my-ml-project")

# Create or get an experiment
experiment = project.get_or_create_experiment("hyperparameter-search")

# Start a run
with experiment.start_run(name="run-001") as run:
    # Log parameters
    run.log_params({
        "learning_rate": 0.001,
        "batch_size": 32,
        "epochs": 100
    })
    
    # Log metrics during training
    for epoch in range(100):
        loss = train_epoch()
        run.log_metric("loss", loss, step=epoch)
        run.log_metric("accuracy", evaluate(), step=epoch)
    
    # Log artifacts
    run.log_artifact("model.pt", "/path/to/model.pt")
    
    # Run automatically completes when exiting the context
```

## Using Raw HTTP API

You can also use the SDK to make raw API calls:

```python
import gradlog

client = gradlog.Client(
    host="https://your-gradlog-instance.com",
    api_key="gl_your_api_key"
)

# Make raw API calls
response = client.get("/api/v1/projects")
projects = response.json()
```

## Environment Variables

The SDK supports configuration via environment variables:

- `GRADLOG_HOST`: The Gradlog server URL
- `GRADLOG_API_KEY`: Your API key

```python
import gradlog

# Will use environment variables
client = gradlog.Client()
```

## License

MIT



# Git backup

This tool is used to backup plainid configuration files to a git repository.  
The main concept is build around versioning of the configuration files, so you can easily rollback to a previous version if needed.  
Versioning is done by tagging every commit with the following syntax: `YYYYMMDD-HHMMSS`.
To avoid merge conflicts we are using technique called `Detached commits when tagging`, which isolates every commit and conflicts free.

## Configuration

Configuration could be provided in a form of a file or environment variables.  
In case of file, it should be placed in the same directory as the binary or home directory and named `.git-backup`.
You can also specify a custom config file path using the `-f` or `--file` flag when running the command.

The configuration file uses YAML format with the following structure:

```yaml
# Git configuration
git:
    repo: "https://github.com/organization/repo.git"
    token: "your-github-token"
    branch: "main"
    delete-temp-on-success: false

# PlainID configuration
plainid:
    base-url: "https://api.plainid.io"
    client-id: "your-client-id"
    client-secret: "your-client-secret"
    envs:
        - id: "environment-id-1"
          workspaces:
              - id: "workspace-id-1"
              - id: "workspace-id-2"
          identities:
              - User
              - Services
        # Use wildcard to backup all workspaces in an environment
        - id: "environment-id-2"
          workspaces:
              - id: "*"
          identities:
              - User
        # Use wildcard to backup all environments
        # - id: "*"
```

### Configuration Options

-   **Git Configuration**:

    -   `git.repo`: The git repository URL where configurations will be stored (HTTPS URL format).
    -   `git.token`: The git token used for authentication.
    -   `git.branch`: The branch where files will be stored (defaults to "main").
    -   `git.delete-temp-on-success`: Boolean flag that controls whether temporary files are deleted after a successful backup operation (defaults to false). When set to true, temporary directories created during the backup process will be automatically cleaned up upon successful completion.

-   **PlainID Configuration**:
    -   `plainid.base-url`: The PlainID API base URL.
    -   `plainid.client-id`: The client ID for PlainID authentication.
    -   `plainid.client-secret`: The client secret for PlainID authentication.
    -   `plainid.envs`: List of environments to backup:
        -   `id`: Environment ID (can be a specific ID or "\*" to match all environments)
        -   `workspaces`: List of workspaces within the environment:
            -   `id`: Workspace ID (can be a specific ID or "\*" to match all workspaces)
        -   `identities`: List of identity types to backup for this environment. (can be "\*" to match all identities).  
            Please notice if you're not using the wildcard "\*" for identities, identities you specify must exist in the PlainID environment, otherwise the backup will fail.

## Usage

Before running the git-backup tool, you need to configure it using one of the following methods:

1. **Configuration file**: Create a `.git-backup` YAML file in the same directory as the binary or in your home directory.

2. **Environment variables**: Set the configuration options as environment variables with underscore separator.

3. **Command-line flags**: Pass the configuration options as flags when executing the tool.

### Example usage:

```bash
# Using configuration file (already set up)
./git-backup

# Using custom configuration file
./git-backup -f /path/to/my-config.yaml
# or
./git-backup --file /path/to/my-config.yaml

# Using environment variables
export git_repo="https://github.com/organization/repo.git"
export git_token="your-github-token"
export git_branch="main"
export plainid_base_url="https://api.plainid.io"
export plainid_client_id="your-client-id"
export plainid_client_secret="your-client-secret"
export plainid_envs='[{"id":"your-environment-id","workspaces":[{"id":"your-workspace-id"}]}]'
export plainid_identities="User, Services"
./git-backup

# Using command-line flags
./git-backup --git.repo="https://github.com/organization/repo.git" \
             --git.token="your-github-token" \
             --git.branch="main" \
             --plainid.base-url="https://api.plainid.io" \
             --plainid.client-id="your-client-id" \
             --plainid.client-secret="your-client-secret" \
             --plainid.envs='[{"id":"your-environment-id","workspaces":[{"id":"your-workspace-id"}]}]' \
             --plainid.identities="User, Services"
```

The tool will authenticate with PlainID, fetch the configuration files for all specified environments and workspaces, and push them to the specified git repository with proper versioning.

### Available Commands

The git-backup tool provides several commands to help you manage your PlainID configurations:

#### backup

The `backup` command fetches the current PlainID configuration for all specified environments and workspaces and creates a new git commit with a tag.

```bash
./git-backup backup
```

This will create a new commit with all PlainID configurations and tag it with the format `YYYYMMDD-HHMMSS`. The commit message will include all environment and workspace IDs that were backed up.

#### restore

note: this is not fully yet implemented.
The `restore` command provides an interactive selection from the 10 most recent backups, allowing you to choose which version to restore:

```bash
./git-backup restore
```

Once a backup is selected, the tool will download the configuration files from the chosen git tag and update the PlainID workspace with these configurations.

For non-interactive mode, you can specify a specific tag to restore using the `--tag` parameter:

```bash
./git-backup restore --tag="20230115-120000"
```

Additionally, you can use the `--target-dir` parameter to check out the configuration into a specified directory without further processing, which allows for manual restoration:

```bash
./git-backup restore --tag="20230115-120000" --target-dir="/path/to/output"
```

When used with `--dry-run`, the tool will only check out the specified configurations into the target directory without processing it further:

```bash
./git-backup restore --tag="20230115-120000" --target-dir="/path/to/output" --dry-run
```

#### list

The `list` command shows the 10 most recent backups without restoring any configuration:

```bash
./git-backup list
```

You can filter backups by environment ID and workspace ID:

```bash
./git-backup list --env-id="your-environment-id" --ws-id="your-workspace-id"
```

This is useful for reviewing available backups before deciding which one to restore. The output shows the timestamp, environment ID, and workspace ID for each backup.

### Dry Run Mode

For both `backup` and `restore` commands, you can use the `--dry-run` flag to test the process without making any actual changes:

```bash
./git-backup backup --dry-run
./git-backup restore --dry-run
```

In dry run mode:

-   For `backup`: The tool will download configuration files from PlainID but won't push them to git
-   For `restore`: The tool will download configuration files from git but won't upload them to PlainID

This is useful for validating configurations and testing the process before making actual changes.

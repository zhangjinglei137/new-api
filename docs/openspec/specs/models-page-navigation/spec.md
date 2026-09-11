# models-page-navigation Specification

## Purpose
Defines the model management page navigation contract: the top-level tab structure (models list, vendors, endpoints, deployments), how vendor and endpoint management are embedded as page tabs instead of menu dialogs, and the persistent model metadata sync entry with its source ordering and default.

## Requirements

### Requirement: Top-level tab navigation
The model management page SHALL expose a top-level tab bar containing exactly four tabs in this order: Models (model list), Vendors (supplier management), Endpoints (endpoint management), and Deployments (deployment management). The default landing tab SHALL be Models.

#### Scenario: Landing on the models page
- **WHEN** an admin opens the model management page without a section
- **THEN** the Models tab is active and the model list is displayed

#### Scenario: Switching tabs
- **WHEN** the admin clicks the Vendors, Endpoints, or Deployments tab
- **THEN** the URL section changes to the corresponding section and the matching content is rendered below the tab bar

### Requirement: Vendor management as a page tab
The vendor management capability SHALL be accessible as the Vendors page tab rather than through the top-right overflow menu. The tab SHALL render a vendor table with the same behaviors as the previous vendor management dialog: paginated listing, keyword search, create/edit, delete with reference counts, and vendor mutation via the shared vendor form.

#### Scenario: Accessing vendor management from the tab
- **WHEN** an admin clicks the Vendors tab
- **THEN** the vendor table is rendered inline and the "Manage Vendors" entry is no longer present in the top-right overflow menu

#### Scenario: Creating a vendor from the Vendors tab
- **WHEN** the admin clicks the Add Vendor action on the Vendors tab
- **THEN** the vendor create dialog opens and a new vendor can be saved

#### Scenario: Vendors tab with pagination and search
- **WHEN** the vendor list exceeds one page or the admin enters a search keyword
- **THEN** the table paginates results and filters by keyword server-side

### Requirement: Endpoint management as a page tab
The endpoint management capability SHALL be accessible as the Endpoints page tab positioned between the Vendors tab and the Deployments tab. The tab SHALL render the endpoint definition table with the same behaviors as the previous endpoint management dialog: listing endpoint definitions, editing fields (HTTP method, path, NPM), and saving updates.

#### Scenario: Accessing endpoint management from the tab
- **WHEN** an admin clicks the Endpoints tab
- **THEN** the endpoint definition table is rendered inline and the "Manage Endpoints" entry is no longer present in the top-right overflow menu

#### Scenario: Editing an endpoint definition
- **WHEN** the admin modifies an endpoint definition field on the Endpoints tab and saves
- **THEN** the update is persisted and the table refreshes with the new values

### Requirement: Overflow menu cleanup
The top-right overflow menu of the model management page SHALL no longer offer "Manage Vendors" or "Manage Endpoints" entries, since both capabilities are now exposed as tabs.

#### Scenario: Menu no longer shows vendor/endpoint entries
- **WHEN** the admin opens the top-right overflow menu on the Models tab
- **THEN** neither "Manage Vendors" nor "Manage Endpoints" appears among the menu items

### Requirement: Persistent model metadata sync entry
The model management page SHALL provide a persistent, directly selectable model metadata sync entry in the primary action area (next to the Add Model button). Clicking it SHALL open the sync wizard with source selection visible first.

#### Scenario: Triggering sync from the primary action area
- **WHEN** the admin clicks the sync entry button in the primary action area
- **THEN** the sync wizard opens and the source selection step is shown before language selection

### Requirement: Sync source ordering and default
The sync source options SHALL be ordered OpenCode Go first, Official Repository second, and Configuration File last (Configuration File remains disabled). OpenCode Go SHALL be the default selected source when the sync wizard opens.

#### Scenario: Default source is OpenCode Go
- **WHEN** the sync wizard opens
- **THEN** OpenCode Go is the pre-selected source option

#### Scenario: Source options ordered with OpenCode Go first
- **WHEN** the sync source options are rendered
- **THEN** OpenCode Go appears first, Official Repository second, and Configuration File last in a disabled state

# AI Application Platform

This context defines the business language of a public multi-tenant AI application SaaS for consumers and small businesses. The platform unifies the commercial and access experience around independently owned AI products.

## Platform and customers

**Platform**:
The public multi-tenant SaaS that provides unified identity, entry, subscription, usage visibility, and lifecycle management for integrated AI products.
_Avoid_: WeKnora Enterprise, RAGFlow Enterprise, universal agent framework

**Billing Customer**:
The individual or small business legally responsible for paying for one Tenant. In the initial model, each Billing Customer owns exactly one Tenant and each Tenant has exactly one Billing Customer.
_Avoid_: Customer, User, member, account

**Tenant**:
The platform's hard security, data, and billing isolation boundary. A Tenant contains multiple Users and all subscriptions, balances, usage, and bills belong to it.
_Avoid_: Organization, workspace, namespace

**Personal Tenant**:
A Tenant created for and owned by an individual Billing Customer during self-registration.
_Avoid_: Personal account, personal workspace

**Business Tenant**:
A Tenant created for and owned by a small-business Billing Customer. Its Owner can invite multiple Users as members.
_Avoid_: Organization, company account, business workspace

**User**:
A human identity that can own a Personal Tenant and can be a member of multiple Tenants.
_Avoid_: Customer, Tenant, member record

**Membership**:
The relationship allowing a User to act within a Tenant under a Platform role. It becomes usable only when active, and revocation immediately removes Tenant and Product Access without deleting the global User.
_Avoid_: User, Product role, Customer

**User Deactivation**:
The disabling of a global User identity after resolving every Tenant it owns. It is separate from leaving a Tenant and from Tenant Erasure.
_Avoid_: Tenant deletion, membership removal

**Tenant Context**:
The explicitly selected and trusted Tenant under whose authority a User performs an operation. It must not be inferred from untrusted product or client identifiers.
_Avoid_: Organization context, implicit current account

**Platform Context**:
A short-lived, audience-bound assertion issued after the Platform validates an active Membership and authorization for a selected Tenant Context. Products receive it only through the trusted Platform edge and network path.
_Avoid_: Client tenant header, Keycloak token, long-lived role token

## Platform access

**Owner**:
The sole Platform role accountable for a Tenant's ownership, deletion, billing authority, and complete access policy.
_Avoid_: Super Admin, Product administrator

**Admin**:
A Platform role that manages Tenant membership, Product purchases, configuration, and Product Access without owning the Tenant.
_Avoid_: Owner, Product administrator

**Billing Member**:
A Platform role that manages payments and can inspect subscriptions, usage, and bills without access to Product Domain Objects.
_Avoid_: Billing admin, accountant User, Product member

**Member**:
A Platform role that can use Products for which the User has received Product Access.
_Avoid_: Customer, subscriber, Product administrator

**Product Access**:
A Tenant-scoped grant allowing a User to enter and use a Product. The Owner receives it automatically after purchase, while other Business Tenant Users require an explicit grant; it does not replace roles or permissions defined inside that Product.
_Avoid_: Product role, Product entitlement

**Authorization Projection**:
A derived representation of active Platform relationships used to answer authorization questions. It is not the source of truth for Membership or Product Access lifecycle.
_Avoid_: Membership database, Product permission model

**Cross-Tenant Sharing**:
Access by a User acting under one Tenant Context to a Product Domain Object owned by another Tenant. It is forbidden in the initial Platform; collaboration requires membership in the owning Tenant.
_Avoid_: Organization sharing, external workspace sharing

## Products

**Product**:
An independently bounded AI application made available through the Platform. A Product retains ownership of its own domain objects and behavior.
_Avoid_: Platform module, shared domain

**Product Version**:
An immutable, deployable identity for a Product release that binds the upstream release, Adapter and Patch Set compatibility, data schema, and built artifact.
_Avoid_: Product, latest image, mutable release

**Migration Class**:
A Product Version declaration describing whether its data change is absent, backward-compatible, forward-only, or destructive and therefore what backup, downtime, restore, and rollback promises are truthful.
_Avoid_: Image rollback, upgrade status

**Product Instance**:
A running, capacity-bounded Cell of one Product Version in an environment. A Shared Product Instance can serve multiple Tenants without becoming any one Tenant's security boundary or an unbounded global singleton.
_Avoid_: Tenant, namespace, Product

**Product Binding**:
The Tenant-owned relationship to a Product Instance, including the server-controlled external Tenant mapping and commercial lifecycle status.
_Avoid_: Product Instance, external Tenant, subscription

**Desired State**:
The Product Binding state requested by the Platform or Tenant: active, suspended, or erased.
_Avoid_: Workflow status, current health

**Observed State**:
The Platform's latest confirmed Product Binding condition: absent, provisioning, active, degraded, suspended, erasing, or erased.
_Avoid_: Desired State, operation result

**Lifecycle Operation**:
A separately identified attempt to move a Product Binding toward its Desired State, with its own type, progress, retry, classified failure, compensation, and human-attention status.
_Avoid_: Product Binding status, Temporal Workflow history

**Cell Capacity Reservation**:
An allocation of a Product Instance's bounded Tenant, storage, vector, background-work, request, ingestion, and database capacity to a Product Binding before placement.
_Avoid_: Usage Reservation, Tenant quota

**Cell Migration**:
An explicit lifecycle operation that exports, verifies, switches, and retains a rollback window while moving a Product Binding between Product Instances.
_Avoid_: Automatic rebalance, Product upgrade

**Product User Binding**:
The mapping from one Platform User to one external Product user within a Product Instance. It is keyed by stable identities rather than email, phone, or username.
_Avoid_: Product Membership, email account match

**Product Membership Binding**:
The mapping from a Platform Membership and Product Binding to the corresponding external Product membership and role. It does not redefine either Platform Role or Product-internal Role.
_Avoid_: Product User Binding, Platform Role

**Product Domain Object**:
A business object whose meaning and lifecycle belong to one Product, such as a WeKnora knowledge base. It is not redefined as a universal Platform object merely to create a unified user interface.
_Avoid_: Platform resource, canonical AI object

**Product Catalog**:
The internally curated set of Products approved and operated by the Platform team.
_Avoid_: Open marketplace, public plugin marketplace

**Product Offer**:
A purchasable commercial offer for one Product that defines its price, entitlements, and included usage allowance. Multiple Product Offers can be billed through the same Tenant experience without becoming one universal Product plan.
_Avoid_: Platform-wide plan, Product, price only

**Data Processing Profile**:
The Product Offer's declared set of approved model Providers, processing regions, content-retention and training-use policies, supported data classes, and subprocessors.
_Avoid_: Model route, privacy-policy link, automatic fallback

**Capability Contract**:
A Platform-owned, product-neutral contract promoted only after multiple Products demonstrate the same stable business meaning. It is not inferred from one Product API or from superficial naming similarity.
_Avoid_: Universal Product API, Adapter method

**AI Runtime Asset**:
A versioned Agent, MCP Server, Skill, or Prompt made discoverable to approved runtime consumers while its Product-level metadata remains owned by the Platform.
_Avoid_: Product Domain Object, Nacos database row

**License Report**:
A Product Version-specific determination of commercial hosting, modification, redistribution, branding, disclosure, and dependency obligations across source, assets, plugins, models, datasets, build inputs, and container bases.
_Avoid_: Repository license file, one-time Product approval

**Lighthouse Product**:
The first Product used to prove the complete customer journey and expose invalid Platform abstractions. WeKnora is the initial Lighthouse Product.
_Avoid_: Reference implementation, mandatory framework

**Identity Binding**:
The link between a Platform User and a stable subject issued by an Identity Provider. Mutable email addresses, phone numbers, and usernames are not identity keys.
_Avoid_: User, email lookup, membership

## Commercial lifecycle

**Subscription**:
A Tenant-owned recurring commercial agreement that grants a defined set of product access, entitlements, and included usage allowance.
_Avoid_: Plan, payment, balance

**Prepaid Usage Balance**:
A Tenant-owned monetary balance denominated in CNY, purchased before consumption and usable across Products when metered usage exceeds an included allowance.
_Avoid_: Postpaid credit, User balance

**Credit Lot**:
A source-preserving portion of Prepaid Usage Balance with its original Payment Order, amount, remainder, currency, expiry, and refundability. Refund and consumption policies operate on Lots rather than an undifferentiated total.
_Avoid_: Aggregate balance, Included Allowance

**Included Allowance**:
A quantity of a specific Meter included with a Product Offer. It cannot be exchanged for a semantically different Meter merely because both are billable.
_Avoid_: Cash balance, universal credit, User quota

**User Budget**:
A Tenant policy that limits how much a User may consume. It is a guardrail over Tenant-owned funds, not money or a separately owned balance.
_Avoid_: User wallet, User credit

## Usage

**Canonical Usage Event**:
An immutable, replayable record of a billable fact emitted by an approved server-side authority. It is scoped to a Tenant, Product, Product Instance, Meter, subject, source, and occurrence time, but does not embed a price or duplicate Organization or Billing Customer identity.
_Avoid_: Client analytics event, invoice line, priced event

**Usage Authority**:
The approved server-side source for a Meter: a trusted Model Gateway for provider usage, a Product Adapter collector for Product-domain usage, or the Billing Control Plane for commercial state. A browser or external client is never a Usage Authority.
_Avoid_: Event caller, frontend meter

**Usage Adjustment**:
An immutable Usage Event that corrects a prior event by referencing it and applying a positive or negative quantity delta with a recorded reason and authority.
_Avoid_: Event update, event deletion, aggregate edit

**Usage Reservation**:
A temporary hold against a Tenant's spendable balance that authorizes a variable-cost operation up to a maximum amount before its final usage is known.
_Avoid_: Charge, settled usage, User Budget

**Usage Settlement**:
The conversion of a Usage Reservation into the actual charge supported by a trusted final Usage Event, followed by release of any unused reserved amount.
_Avoid_: Authorization, estimate, reservation

**Usage Subject**:
The resource or party whose activity is measured within a Tenant. It is distinct from the optional actor User and never replaces the Tenant as the billing boundary.
_Avoid_: User, Billing Customer, actor

**Rated Result**:
The monetary result of applying the Price version effective at a Usage Event's occurrence time to its fixed-precision quantity. It records the Price version and explicit rounding rule used.
_Avoid_: Usage Event, floating-point estimate

**Late Usage**:
A Canonical Usage Event ingested after its occurrence time. Events no more than 24 hours late are rated automatically against the historical Price version; older events require reconciliation review.
_Avoid_: Current-period usage, correction

**Plan Change**:
A change to a Tenant's Subscription terms, entitlements, or included allowance that does not transfer or redefine ownership of Product Domain Objects.
_Avoid_: Product upgrade, migration

**Product Version Upgrade**:
A change to the running version of a Product that can require data migration and compatibility validation while preserving Product Domain Objects.
_Avoid_: Plan upgrade, subscription change

## Payment and service termination

**Payment Order**:
A Tenant-owned request to fund a purchase through an external payment channel. Its payment state is separate from fulfillment into the Tenant's balance.
_Avoid_: Balance entry, invoice, Subscription

**Payment Provider**:
An external payment channel that confirms funds and refunds through a Platform-owned contract. WeChat Pay and Alipay are the initial Payment Providers.
_Avoid_: Billing engine, balance ledger

**Payment Journal**:
The Platform-owned immutable history of Payment Orders, Provider Transactions, refunds, and confirmed money movement. It is the source of truth for real funds, not for usage credit balances.
_Avoid_: OpenMeter balance, invoice, mutable payment status

**Provider Transaction**:
The Payment Provider's independently identified record of a charge or refund linked to a Platform Payment Order.
_Avoid_: Payment Order, balance entry

**Paid Payment**:
A Payment Order for which the external payment channel has confirmed successful receipt of funds, whether or not the purchased value has been fulfilled.
_Avoid_: Fulfilled payment, credited balance

**Payment Fulfillment**:
The idempotent application of a Paid Payment's purchased value to the Tenant's commercial state.
_Avoid_: Payment confirmation, callback receipt

**Refund Order**:
A request to return an eligible amount through the original Payment Provider. Its amount is reserved against concurrent consumption before the Provider is called and is reversed in the balance only after Provider confirmation.
_Avoid_: Direct balance edit, refund callback

**Billing Customer Tax Profile**:
The verified invoicing identity and tax information used to request an electronic invoice for confirmed and fulfilled payments.
_Avoid_: User profile, Payment Provider identity

**Invoice Request**:
A Billing Customer request to issue, void, or reverse an electronic invoice against eligible confirmed and fulfilled payment facts.
_Avoid_: OpenMeter usage invoice, Payment Order

**Reconciliation Case**:
An independently tracked discrepancy between external money or canonical usage facts and their Platform, fulfillment, rating, or ledger representations. It is resolved through new immutable corrective events rather than direct state edits.
_Avoid_: Log alert, balance edit, ignored mismatch

## Governance

**Audit Event**:
An immutable, content-minimized record of a security-relevant or financially relevant action, its authority, Tenant Context, stable resource identity, outcome, and evidence references.
_Avoid_: Debug log, Trace, Usage Event

**Production Gate**:
An evidence-backed condition that a Product Version and Platform release must satisfy before serving paying Tenants. A health check, unit test, or Workflow completion cannot substitute for a required Gate.
_Avoid_: Deployment success, smoke test

**Emergency Access**:
Exceptional, time-bounded Platform-operator access during an active security incident, followed by notification, review, and immutable Audit. It is not standing System Admin access or silent User impersonation.
_Avoid_: Support login, permanent admin

**Quarantine**:
A reversible security state that stops new traffic, background work, access grants, provisioning, and upgrades while preserving Product data and evidence for investigation.
_Avoid_: Tenant Erasure, Product deletion

**Tenant Export**:
A Product Adapter-produced package and manifest containing the Tenant-owned Product data, Product and schema versions, object checksums, export time, and completeness evidence.
_Avoid_: Cross-Product conversion, database backup

**Read-only Retention**:
The 30-day period after paid service ends during which a Tenant can inspect and export Product Domain Objects or reactivate service but cannot create new cost-bearing work.
_Avoid_: Active subscription, deletion grace with full access

**Tenant Erasure**:
The auditable removal of a terminated Tenant's Product Domain Objects, files, vectors, queued work, credentials, and caches after Read-only Retention ends. Legally required financial and audit records are governed separately.
_Avoid_: Tenant soft delete, access suspension

**Erasure Record**:
The immutable, data-minimized evidence that every required Platform and Product store completed or scheduled its part of Tenant Erasure. A Workflow success marker alone is not an Erasure Record.
_Avoid_: Workflow completion, deletion log with business data

**Restore Rehearsal**:
A verified recovery of the complete Cell recovery set into an isolated environment, measuring actual RPO and RTO rather than merely confirming that backup files exist.
_Avoid_: Backup job success, image rollback

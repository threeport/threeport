# Feedback Loop

This document outlines the systems that users and developers leverage to
clearly and easily understand what is happening in QleetOS.  The asyncronous
manner of system reconcilation requires an appropriate system for relaying
information back to clients, operators and extenders of the system.

## Client Message API

The Client Message API allows controllers in the Threeport control plane to post
status messages about the work they are doing.  That allows the clients to get
those messages and stay up-to-date by querying the API for the latest messages
about a topic they're interested in.

This also allows us to deploy controllers that watch for those messages and push
them out to other systems so they don't have to poll the API for new messages
that may or may not exist.

A `ClientMessage` object contains the following attributes:

* ControllerName
* ReconcilerName
* ObjectName
* ObjectID
* MessageBody
* MessageCode

## Metrics

Threeport uses [prometheus](https://prometheus.io/) to gather and expose
metrics in the system.  These metrics provide insight into compute usage as well
as application performance.

If users have an existing metrics system that supports prometheus metrics (most
do) they can integrate their Threeport installation with that provider and take
advantage of integrations they already use.

## Logs

Threeport system components write logs to standard out so as to provide
developers with the granular information they need to troubleshoot the system.
This logging system is also designed to be used by tenant workloads so that
maintainers and operators of the software can gain access to log output from
their workloads.

Threeport does not provide any log storage back end.  There are numerous
excellent managed, supported and open source solutions for this purpose.
Threeport integrates with these systems so that users can leverage existing
systems or choose those that best fit their needs and budget.


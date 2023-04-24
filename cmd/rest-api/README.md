# Threeport RESTful API

The heart of the threeport control plane.

Here you will find the main package for the threeport API.  It is the
interaction point for clients to use the threeport control plane.  The API is
responsible for persisting desired state of the system as expressed by users and
external systems, and for notifying the controllers in the system to reconcile
state accordingly.  The controllers read and write to and from the API as needed
and all coordination between controllers happen through this API - they never
interact directly with each other.  The API is built using the [echo
framework](https://github.com/labstack/echo).


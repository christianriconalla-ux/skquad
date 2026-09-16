"use client";

import { ApiState, LLMProvider } from "../lib/api";
import { StateNotice, providerModels } from "./shared";
import { ListHeader, Modal, useModalForm } from "./Modal";

type ProviderForm = { name: string; kind: string; base_url: string; api_key_ref: string; default_model: string; models: string };

// LLM providers are not grantable resources — a squad picks one and its agents
// inherit it — so they live in their own main-menu section rather than under
// Resources or Admin.
export function ProvidersSection({
  providers,
  providerForm,
  setProviderForm,
  onCreateProvider,
  onDeprecateProvider,
}: {
  providers: ApiState<LLMProvider[]>;
  providerForm: ProviderForm;
  setProviderForm: (form: ProviderForm) => void;
  onCreateProvider: () => Promise<string | null>;
  onDeprecateProvider: (id: string) => void;
}) {
  const providerItems = providers.data || [];
  const providerModal = useModalForm(onCreateProvider);
  return (
    <div className="section-stack">
      <ListHeader
        title="LLM Providers"
        detail="Squads choose one of these providers when they are created, and their agents pick from its models."
      >
        <button type="button" className="primary" onClick={providerModal.show}>
          + Register provider
        </button>
      </ListHeader>
      <StateNotice state={providers} empty="No providers registered" />
      {providerItems.length > 0 && (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Kind</th>
                <th>Models</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {providerItems.map((provider) => (
                <tr key={provider.id}>
                  <td>
                    <strong>{provider.name}</strong>
                    <small>{provider.id}</small>
                  </td>
                  <td>{provider.kind}</td>
                  <td>{providerModels(provider).join(", ") || "-"}</td>
                  <td>{provider.status}</td>
                  <td>
                    {provider.status === "active" && (
                      <button type="button" className="secondary small" onClick={() => onDeprecateProvider(provider.id)}>
                        Deprecate
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <Modal {...providerModal.props} title="Register LLM provider" submitLabel="Register provider">
        <label>
          Name
          <input value={providerForm.name} onChange={(event) => setProviderForm({ ...providerForm, name: event.target.value })} required />
        </label>
        <label>
          Kind
          <input value={providerForm.kind} onChange={(event) => setProviderForm({ ...providerForm, kind: event.target.value })} required />
        </label>
        <label>
          Base URL
          <input value={providerForm.base_url} onChange={(event) => setProviderForm({ ...providerForm, base_url: event.target.value })} required />
        </label>
        <label>
          API key ref
          <input value={providerForm.api_key_ref} onChange={(event) => setProviderForm({ ...providerForm, api_key_ref: event.target.value })} />
        </label>
        <label>
          Default model
          <input value={providerForm.default_model} onChange={(event) => setProviderForm({ ...providerForm, default_model: event.target.value })} />
        </label>
        <label>
          Models JSON array
          <textarea value={providerForm.models} onChange={(event) => setProviderForm({ ...providerForm, models: event.target.value })} rows={3} placeholder='["gpt-4.1-mini"]' />
        </label>
      </Modal>
    </div>
  );
}
